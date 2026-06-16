package account

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	ccov1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/controller"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/osdCloud"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/provider/pagerduty"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AWS IAM Credential Architecture for OSD/ROSA Classic Clusters
//
// There are two IAM users managed per cluster, with distinct roles:
//
// osdCcsAdmin (CCS/BYOC clusters only):
//   - Created by the customer with AdministratorAccess policy attached.
//   - Credentials are provided to Red Hat and stored in the "byoc" secret on
//     Hive (referenced by AccountClaim.Spec.BYOCSecretRef).
//   - The aws-account-operator uses these credentials to:
//     1. Create the ManagedOpenShift-Support-<suffix> IAM role in the customer's
//     account, with a trust policy allowing RH-SRE-CCS-Access to assume it.
//     2. Assume that role to create/manage the osdManagedAdmin IAM user.
//   - The operator re-reads the byoc secret on every reconciliation (no caching),
//     so updating the secret after rotation is sufficient — no operator restart needed.
//   - Rotating the access key does NOT invalidate the ManagedOpenShift-Support
//     trust policy, because the policy references the IAM user ARN (which doesn't
//     change), not the access key.
//
// osdManagedAdmin (all non-STS clusters):
//   - Created by the aws-account-operator (using osdCcsAdmin creds on CCS, or
//     OrganizationAccountAccessRole on non-CCS).
//   - Credentials stored in the account secret on Hive and synced to the cluster
//     as kube-system/aws-creds via SyncSet.
//   - The Cloud Credential Operator (CCO) uses these credentials to fulfill
//     CredentialRequests — creating per-operator IAM users/policies and secrets
//     (e.g., openshift-image-registry, openshift-ingress, openshift-machine-api).
//   - After rotation, CredentialRequest secrets must be deleted so CCO recreates
//     them with the new credentials.
//
// SRE/Backplane access path (independent of both users):
//   - Backplane authenticates via OCM JWT → SRE-Support-Role → role chain →
//     ManagedOpenShift-Support-<suffix> in the customer account.
//   - This path does NOT read osdCcsAdmin or osdManagedAdmin credentials.
//   - The dependency is indirect: osdCcsAdmin created the infrastructure (roles)
//     that backplane later assumes.
func newCmdAWSCreds(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "aws-creds",
		Short:             "Diagnose and manage AWS IAM credentials for a cluster",
		Long:              "Subcommands for inspecting and rotating AWS IAM credentials, Hive secrets, and CredentialRequests.",
		DisableAutoGenTag: true,
	}

	cmd.AddCommand(newCmdAWSCredsSnapshot(streams))
	cmd.AddCommand(newCmdAWSCredsRotate(streams))

	return cmd
}

type awsCredsOptions struct {
	clusterID     string
	profile       string
	awsUseEnv     bool
	reason        string
	adminUsername string
	hiveOcmUrl    string
	logLevel      string

	log *logrus.Logger
	genericclioptions.IOStreams
}

func newAWSCredsLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(os.Stderr)
	l.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "15:04:05",
		ForceColors:     true,
	})
	l.SetLevel(logrus.InfoLevel)
	return l
}

func (o *awsCredsOptions) addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.clusterID, "cluster-id", "C", "", "(Required) OCM internal or external cluster ID")
	cmd.Flags().StringVarP(&o.profile, "aws-profile", "p", "", "AWS profile for role chaining. If omitted, tries backplane then falls back to default AWS credential chain")
	cmd.Flags().BoolVar(&o.awsUseEnv, "aws-use-env", false, "Use AWS credentials from environment variables (e.g. after rh-aws-saml-login), skipping backplane")
	cmd.Flags().StringVarP(&o.reason, "reason", "r", "", "(Required) Elevation reason, usually a Jira ticket ID")
	cmd.Flags().StringVar(&o.adminUsername, "admin-username", "", "Override the osdManagedAdmin IAM username. Only needed if auto-detection fails (e.g. custom or legacy username)")
	cmd.Flags().StringVar(&o.hiveOcmUrl, "hive-ocm-url", "", "OCM environment for Hive operations (aliases: production, staging, integration)")
	cmd.Flags().StringVarP(&o.logLevel, "log-level", "l", "info", "Log level: debug, info, warn, error")

	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("reason")

	hideIrrelevantGlobalFlags(cmd)
}

const subcommandUsageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

// hideIrrelevantGlobalFlags replaces the default usage template to suppress inherited global flags.
func hideIrrelevantGlobalFlags(cmd *cobra.Command) {
	cmd.SetUsageTemplate(subcommandUsageTemplate)
}

var (
	jiraTicketPattern    = regexp.MustCompile(`(OHSS|SREP|ROSAENG|OSD|SDE|CSSRE|MNGT)-\d+`)
	pdIncidentIDPattern  = regexp.MustCompile(`(?:pagerduty\.com/incidents/|^|\s)([A-Z0-9]{10,})`)
	pdIncidentNumPattern = regexp.MustCompile(`[Ii]ncident\s*#?(\d{4,})`)
)

// validateReason checks that the --reason flag contains a Jira ticket or PD incident
// associated with the target cluster, returning true if any warnings were issued.
func (o *awsCredsOptions) validateReason(rc *resolvedCluster) bool {
	jiraTickets := jiraTicketPattern.FindAllString(o.reason, -1)

	var pdIncidents []string
	for _, match := range pdIncidentIDPattern.FindAllStringSubmatch(o.reason, -1) {
		pdIncidents = append(pdIncidents, match[1])
	}
	for _, match := range pdIncidentNumPattern.FindAllStringSubmatch(o.reason, -1) {
		pdIncidents = append(pdIncidents, match[1])
	}
	warned := false

	if len(jiraTickets) == 0 && len(pdIncidents) == 0 {
		o.log.Warn("The --reason does not appear to contain a Jira ticket (e.g., OHSS-1234) or PagerDuty incident.")
		o.log.Warn("Accepted PD formats: incident ID (Q3V1PFRGY1YT7T), URL (https://redhat.pagerduty.com/incidents/...), or Incident #1234567")
		return true
	}

	if len(jiraTickets) > 0 {
		if o.validateJiraTickets(rc, jiraTickets) {
			warned = true
		}
	}
	if len(pdIncidents) > 0 {
		if o.validatePDIncidents(rc, pdIncidents) {
			warned = true
		}
	}
	return warned
}

// validateJiraTickets checks whether referenced Jira tickets are associated with the cluster.
func (o *awsCredsOptions) validateJiraTickets(rc *resolvedCluster, tickets []string) bool {
	o.log.WithField("count", len(tickets)).Debug("Validating Jira ticket references from reason")
	warned := false

	clusterIssues, err := utils.GetJiraIssuesForCluster(rc.internalID, rc.cluster.ExternalID(), "")
	if err != nil {
		o.log.WithError(err).Debug("Could not query Jira for cluster-associated tickets (token may not be configured)")
		return false
	}

	clusterTicketKeys := map[string]bool{}
	for _, issue := range clusterIssues {
		clusterTicketKeys[issue.Key] = true
	}

	for _, ticket := range tickets {
		if !clusterTicketKeys[ticket] {
			o.log.WithField("ticket", ticket[:min(len(ticket), 4)]+"...").Warn("Ticket not found in Jira issues associated with this cluster.")
			o.log.Warn("Verify you are operating on the correct cluster. Check with: osdctl cluster context -C " + rc.internalID)
			warned = true
		}
	}
	return warned
}

// validatePDIncidents checks whether referenced PagerDuty incidents are active alerts for the cluster.
func (o *awsCredsOptions) validatePDIncidents(rc *resolvedCluster, incidents []string) bool {
	o.log.WithField("count", len(incidents)).Debug("Validating PagerDuty incident references from reason")
	warned := false

	baseDomain := rc.cluster.DNS().BaseDomain()
	if baseDomain == "" {
		o.log.Debug("Cluster has no baseDomain, skipping PD validation")
		return false
	}

	pdClient := pagerduty.NewClient().WithBaseDomain(baseDomain)
	if _, err := pdClient.Init(); err != nil {
		o.log.WithError(err).Debug("Could not initialize PagerDuty client (token may not be configured)")
		return false
	}

	serviceIDs, err := pdClient.GetPDServiceIDs()
	if err != nil {
		o.log.WithError(err).Debug("Could not query PagerDuty services for cluster")
		return false
	}

	if len(serviceIDs) == 0 {
		o.log.Debug("No PagerDuty services found for cluster baseDomain")
		return false
	}

	clusterAlerts, err := pdClient.GetFiringAlertsForCluster(serviceIDs)
	if err != nil {
		o.log.WithError(err).Debug("Could not query PagerDuty alerts for cluster")
		return false
	}

	clusterIncidentIDs := map[string]bool{}
	for _, alerts := range clusterAlerts {
		for _, alert := range alerts {
			clusterIncidentIDs[alert.ID] = true
		}
	}

	for _, incident := range incidents {
		if !clusterIncidentIDs[incident] {
			redacted := incident
			if len(redacted) > 6 {
				redacted = redacted[:3] + "..." + redacted[len(redacted)-3:]
			}
			o.log.WithField("incident", redacted).Warn("PagerDuty incident not found in active alerts for this cluster.")
			o.log.Warn("Verify you are operating on the correct cluster. Check with: osdctl cluster context -C " + rc.internalID)
			warned = true
		}
	}
	return warned
}

// confirmCluster displays cluster details and prompts the user to confirm they are operating on the correct cluster.
func (o *awsCredsOptions) confirmCluster(rc *resolvedCluster) bool {
	ccsStr := "No"
	if rc.isCCS {
		ccsStr = "Yes"
	}
	fmt.Fprintf(o.Out, "\n==========================================================================\n")
	fmt.Fprintf(o.Out, " Cluster: %s (%s)\n", rc.cluster.Name(), rc.internalID)
	fmt.Fprintf(o.Out, " External ID: %s\n", rc.cluster.ExternalID())
	fmt.Fprintf(o.Out, " CCS/BYOC: %s\n", ccsStr)
	fmt.Fprintf(o.Out, " Reason: %s\n", o.reason)
	fmt.Fprintf(o.Out, "==========================================================================\n")

	warned := o.validateReason(rc)

	if warned {
		fmt.Fprintf(o.Out, "\nThe ticket/incident in --reason was not found to be associated with this cluster.\nAre you sure you want to continue? ")
	} else {
		fmt.Fprintf(o.Out, "\nIs this the correct cluster? ")
	}
	return utils.ConfirmPrompt()
}

// validate parses the log level and validates flag constraints common to all aws-creds subcommands.
func (o *awsCredsOptions) validate(cmd *cobra.Command, _ []string) error {
	level, err := logrus.ParseLevel(o.logLevel)
	if err != nil {
		return fmt.Errorf("invalid --log-level '%s': valid values are debug, info, warn, error", o.logLevel)
	}
	o.log.SetLevel(level)

	if o.awsUseEnv && o.profile != "" {
		return cmdutil.UsageErrorf(cmd, "--aws-use-env and --aws-profile are mutually exclusive")
	}

	if o.adminUsername != "" && !strings.HasPrefix(o.adminUsername, common.OSDManagedAdminIAM) {
		return cmdutil.UsageErrorf(cmd, "admin-username must start with %s", common.OSDManagedAdminIAM)
	}

	if o.hiveOcmUrl != "" {
		if _, err := utils.ValidateAndResolveOcmUrl(o.hiveOcmUrl); err != nil {
			return fmt.Errorf("invalid --hive-ocm-url: %w", err)
		}
	}

	return nil
}

type resolvedCluster struct {
	ocmConn       *sdk.Connection
	cluster       *cmv1.Cluster
	internalID    string
	isCCS         bool
	accountID     string
	suffixLabel   string
	claimName     string
	adminUsername string
	account       *awsv1alpha1.Account
	awsClient     awsprovider.Client
	hiveClient    client.Client
	managedClient client.Client
}

// identifyCluster performs the lightweight OCM lookup and prompts the user
// to confirm they are operating on the correct cluster before any expensive
// operations (hive connection, AWS client setup, etc.).
func (o *awsCredsOptions) identifyCluster() (*resolvedCluster, error) {
	o.log.Info("Creating OCM connection")
	ocmConn, err := utils.CreateConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to create OCM connection: %w", err)
	}

	o.log.WithField("cluster", o.clusterID).Info("Fetching cluster info from OCM")
	cluster, err := utils.GetClusterAnyStatus(ocmConn, o.clusterID)
	if err != nil {
		ocmConn.Close()
		return nil, fmt.Errorf("failed to get cluster %s from OCM: %w", o.clusterID, err)
	}

	if o.clusterID != cluster.ID() && o.clusterID != cluster.ExternalID() {
		ocmConn.Close()
		return nil, fmt.Errorf("cluster id '%s' does not match internal '%s' or external '%s' IDs — aliases are not allowed", o.clusterID, cluster.ID(), cluster.ExternalID())
	}
	internalID := cluster.ID()
	o.log.WithFields(logrus.Fields{"name": cluster.Name(), "id": internalID}).Info("Cluster resolved")

	isCCS, err := utils.IsClusterCCS(ocmConn, internalID)
	if err != nil {
		ocmConn.Close()
		return nil, fmt.Errorf("failed to check CCS status: %w", err)
	}

	rc := &resolvedCluster{
		ocmConn:    ocmConn,
		cluster:    cluster,
		internalID: internalID,
		isCCS:      isCCS,
	}

	if !o.confirmCluster(rc) {
		ocmConn.Close()
		return nil, fmt.Errorf("operation cancelled by user")
	}

	return rc, nil
}

// resolveCluster completes the cluster setup after user confirmation:
// connects to Hive, resolves account claim, sets up AWS and managed clients.
func (o *awsCredsOptions) resolveCluster(ctx context.Context, rc *resolvedCluster) error {
	o.log.Info("Connecting to Hive cluster")
	hiveKubeClient, err := o.connectHive(rc.ocmConn, rc.internalID)
	if err != nil {
		return fmt.Errorf("failed to connect to hive: %w", err)
	}
	rc.hiveClient = hiveKubeClient

	o.log.Info("Resolving account claim from Hive")
	claimName, err := o.resolveAccountClaimName(ctx, hiveKubeClient, rc.ocmConn, rc.internalID)
	if err != nil {
		return fmt.Errorf("failed to resolve account claim: %w", err)
	}
	rc.claimName = claimName
	o.log.WithField("claim", claimName).Info("Account claim resolved")

	o.log.WithField("account", claimName).Info("Fetching Account CR")
	account, err := k8s.GetAWSAccount(ctx, hiveKubeClient, common.AWSAccountNamespace, claimName)
	if err != nil {
		return fmt.Errorf("failed to get Account CR '%s': %w", claimName, err)
	}

	if account.Spec.ManualSTSMode {
		return fmt.Errorf("account %s is STS — no IAM user credentials to manage", claimName)
	}
	rc.account = account

	rc.accountID = account.Spec.AwsAccountID
	suffixLabel, ok := account.Labels["iamUserId"]
	if !ok {
		return fmt.Errorf("no iamUserId label on Account CR %s", claimName)
	}
	rc.suffixLabel = suffixLabel

	adminUsername := o.adminUsername
	if adminUsername == "" {
		adminUsername = common.OSDManagedAdminIAM + "-" + suffixLabel
	}
	rc.adminUsername = adminUsername
	o.log.WithFields(logrus.Fields{"aws_account": rc.accountID, "admin_user": adminUsername}).Info("Account info resolved")

	o.log.Info("Setting up AWS client")
	awsClient, err := o.buildAWSClient(ctx, rc.ocmConn, rc.cluster, hiveKubeClient, account, rc.accountID, suffixLabel)
	if err != nil {
		return fmt.Errorf("failed to set up AWS client: %w", err)
	}
	rc.awsClient = awsClient
	o.log.Info("AWS client ready")

	o.log.Info("Connecting to managed cluster via backplane")
	managedClient, err := o.connectManagedCluster(rc.internalID)
	if err != nil {
		o.log.WithError(err).Warn("Could not connect to managed cluster — credential request diagnostics will be skipped")
	}
	rc.managedClient = managedClient

	return nil
}

// toCredsInput converts a resolvedCluster into the controller.AWSCredsInput used by DiagnoseCredentials.
func (rc *resolvedCluster) toCredsInput(log *logrus.Logger, out io.Writer) *controller.AWSCredsInput {
	return &controller.AWSCredsInput{
		ClusterID:         rc.internalID,
		ClusterName:       rc.cluster.Name(),
		ClusterExternalID: rc.cluster.ExternalID(),
		IsCCS:             rc.isCCS,
		AWSAccountID:      rc.accountID,
		AccountCRName:     rc.claimName,
		Account:           rc.account,
		AdminUsername:     rc.adminUsername,
		AwsClient:         rc.awsClient,
		HiveKubeClient:    rc.hiveClient,
		ManagedClient:     rc.managedClient,
		Log:               log,
		Out:               out,
	}
}

// connectHive establishes a k8s client to the Hive cluster, using the --hive-ocm-url override if specified.
func (o *awsCredsOptions) connectHive(ocmConn *sdk.Connection, clusterID string) (client.Client, error) {
	elevationMsg := fmt.Sprintf("Elevation required for aws-creds on %s", clusterID)

	if o.hiveOcmUrl != "" {
		resolvedURL, err := utils.ValidateAndResolveOcmUrl(o.hiveOcmUrl)
		if err != nil {
			return nil, fmt.Errorf("invalid --hive-ocm-url: %w", err)
		}

		hiveOCM, err := utils.CreateConnectionWithUrl(resolvedURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create hive OCM connection (URL: %s): %w", resolvedURL, err)
		}
		defer hiveOCM.Close()

		hiveCluster, err := utils.GetHiveClusterWithConn(clusterID, ocmConn, hiveOCM)
		if err != nil {
			return nil, fmt.Errorf("failed to get hive cluster (URL: %s): %w", resolvedURL, err)
		}

		o.log.WithFields(logrus.Fields{"hive": hiveCluster.Name(), "ocm_url": resolvedURL}).Info("Connecting to Hive cluster")

		hiveClient, err := k8s.NewAsBackplaneClusterAdminWithConn(
			hiveCluster.ID(),
			client.Options{Scheme: scheme.Scheme},
			hiveOCM,
			o.reason,
			elevationMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create hive k8s client (URL: %s): %w", resolvedURL, err)
		}
		return hiveClient, nil
	}

	hiveCluster, err := utils.GetHiveCluster(clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get hive cluster: %w", err)
	}

	o.log.WithField("hive", hiveCluster.Name()).Info("Connecting to Hive cluster")

	hiveKubeCli, _, _, err := common.GetKubeConfigAndClient(hiveCluster.ID(), o.reason, elevationMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to create hive k8s client: %w", err)
	}
	return hiveKubeCli, nil
}

// resolveAccountClaimName resolves the Account CR name from the AccountClaim on Hive,
// falling back to the OCM cluster resources API if the claim is not directly accessible.
func (o *awsCredsOptions) resolveAccountClaimName(ctx context.Context, hiveClient client.Client, ocmConn *sdk.Connection, clusterID string) (string, error) {
	accountClaim, err := k8s.GetAccountClaimFromClusterID(ctx, hiveClient, clusterID)
	if err == nil && accountClaim != nil && accountClaim.Spec.AccountLink != "" {
		return accountClaim.Spec.AccountLink, nil
	}

	liveResponse, err := ocmConn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Resources().Live().Get().Send()
	if err != nil {
		return "", fmt.Errorf("failed to fetch cluster resources from OCM: %w", err)
	}

	respBody := liveResponse.Body().Resources()
	awsAccountClaim, ok := respBody["aws_account_claim"]
	if !ok {
		return "", fmt.Errorf("cluster does not have an AccountClaim in OCM resources")
	}

	var claimJSON map[string]any
	if err := json.Unmarshal([]byte(awsAccountClaim), &claimJSON); err != nil {
		return "", fmt.Errorf("failed to parse account claim JSON: %w", err)
	}

	spec, ok := claimJSON["spec"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("account claim has no spec")
	}

	accountLink, ok := spec["accountLink"].(string)
	if !ok || accountLink == "" {
		return "", fmt.Errorf("account claim has no accountLink")
	}

	return accountLink, nil
}

// buildAWSClient creates an AWS client using either:
// 1. --aws-profile <name>: explicit local AWS config profile
// 2. --aws-use-env: default AWS credential chain (env vars from rh-aws-saml-login, ~/.aws/config)
// 3. No flags: backplane (default), falling back to default credential chain
func (o *awsCredsOptions) buildAWSClient(ctx context.Context, ocmConn *sdk.Connection, cluster *cmv1.Cluster, hiveClient client.Client, account *awsv1alpha1.Account, accountID, suffixLabel string) (awsprovider.Client, error) {
	if o.profile != "" {
		return o.buildAWSClientViaProfile(ctx, hiveClient, account, accountID, suffixLabel)
	}

	if !o.awsUseEnv {
		client, err := o.buildAWSClientViaBackplane(ctx, ocmConn, cluster)
		if err != nil {
			o.log.WithError(err).Warn("Backplane credential retrieval failed, falling back to default AWS credential chain")
			return o.buildAWSClientViaDefaultChain(ctx, hiveClient, account, accountID, suffixLabel)
		}
		return client, nil
	}

	o.log.Info("Using environment AWS credentials (--aws-use-env)")
	return o.buildAWSClientViaEnv(ctx, hiveClient, account, accountID, suffixLabel)
}

// buildAWSClientViaBackplane obtains AWS credentials through OCM backplane without a local AWS profile.
func (o *awsCredsOptions) buildAWSClientViaBackplane(ctx context.Context, ocmConn *sdk.Connection, cluster *cmv1.Cluster) (awsprovider.Client, error) {
	o.log.Info("Using backplane for AWS credentials (no --aws-profile or --aws-use-env specified)")

	cfg, err := osdCloud.CreateAWSV2Config(ocmConn, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS credentials via backplane: %w", err)
	}

	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve backplane AWS credentials: %w", err)
	}

	return awsprovider.NewAwsClientWithInput(&awsprovider.ClientInput{
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		Region:          "us-east-1",
	})
}

// buildAWSClientViaProfile creates an AWS client using a local profile with manual
// role chaining through SRE access, jump, and support roles.
func (o *awsCredsOptions) buildAWSClientViaProfile(ctx context.Context, hiveClient client.Client, account *awsv1alpha1.Account, accountID, suffixLabel string) (awsprovider.Client, error) {
	awsSetupClient, err := awsprovider.NewAwsClient(o.profile, "us-east-1", "")
	if err != nil {
		return nil, fmt.Errorf("failed to create initial AWS client with profile '%s': %w", o.profile, err)
	}
	return o.roleChainToSupport(ctx, awsSetupClient, hiveClient, account, accountID, suffixLabel)
}

// roleChainToSupport takes an initial AWS client (from any source: profile, env vars, default chain)
// and role-chains through SRE access, jump, and support roles to reach the cluster's AWS account.
func (o *awsCredsOptions) roleChainToSupport(ctx context.Context, awsSetupClient awsprovider.Client, hiveClient client.Client, account *awsv1alpha1.Account, accountID, suffixLabel string) (awsprovider.Client, error) {
	callerIdentity, err := awsSetupClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS caller identity: %w", err)
	}
	roleSessionName := getSessionNameFromUserId(*callerIdentity.UserId)
	timeout := awsSdk.Int32(900)

	var credKeyID, credSecretKey, credToken *string

	if account.Spec.BYOC {
		cm := &corev1.ConfigMap{}
		if err := hiveClient.Get(ctx, client.ObjectKey{
			Name: common.DefaultConfigMap, Namespace: common.AWSAccountNamespace,
		}, cm); err != nil {
			return nil, fmt.Errorf("failed to get aws-account-operator configmap: %w", err)
		}

		sreAccessARN := cm.Data["CCS-Access-Arn"]
		if sreAccessARN == "" {
			return nil, fmt.Errorf("CCS-Access-Arn missing from configmap")
		}
		jumpARN := cm.Data["support-jump-role"]
		if jumpARN == "" {
			return nil, fmt.Errorf("support-jump-role missing from configmap")
		}

		srepCreds, err := awsprovider.GetAssumeRoleCredentials(awsSetupClient, timeout, &roleSessionName, &sreAccessARN)
		if err != nil {
			return nil, fmt.Errorf("failed to assume SRE access role: %w", err)
		}

		srepClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.ClientInput{
			AccessKeyID: *srepCreds.AccessKeyId, SecretAccessKey: *srepCreds.SecretAccessKey,
			SessionToken: *srepCreds.SessionToken, Region: "us-east-1",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create SRE access role client: %w", err)
		}

		jumpCreds, err := awsprovider.GetAssumeRoleCredentials(srepClient, timeout, &roleSessionName, &jumpARN)
		if err != nil {
			return nil, fmt.Errorf("failed to assume jump role: %w", err)
		}

		jumpClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.ClientInput{
			AccessKeyID: *jumpCreds.AccessKeyId, SecretAccessKey: *jumpCreds.SecretAccessKey,
			SessionToken: *jumpCreds.SessionToken, Region: "us-east-1",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create jump role client: %w", err)
		}

		roleArn := awsSdk.String(fmt.Sprintf("arn:aws:iam::%s:role/ManagedOpenShift-Support-%s", accountID, suffixLabel))
		creds, err := awsprovider.GetAssumeRoleCredentials(jumpClient, timeout, &roleSessionName, roleArn)
		if err != nil {
			return nil, fmt.Errorf("failed to assume ManagedOpenShift-Support role: %w", err)
		}
		credKeyID = creds.AccessKeyId
		credSecretKey = creds.SecretAccessKey
		credToken = creds.SessionToken
	} else {
		roleArn := awsSdk.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, awsv1alpha1.AccountOperatorIAMRole))
		creds, err := awsprovider.GetAssumeRoleCredentials(awsSetupClient, timeout, &roleSessionName, roleArn)
		if err != nil {
			return nil, fmt.Errorf("failed to assume OrganizationAccountAccessRole: %w", err)
		}
		credKeyID = creds.AccessKeyId
		credSecretKey = creds.SecretAccessKey
		credToken = creds.SessionToken
	}

	return awsprovider.NewAwsClientWithInput(&awsprovider.ClientInput{
		AccessKeyID: *credKeyID, SecretAccessKey: *credSecretKey,
		SessionToken: *credToken, Region: "us-east-1",
	})
}

// resolveForCRSecrets connects only to Hive (for account secret timestamp)
// and the managed cluster (for CR secrets). Skips AWS client setup entirely.
func (o *awsCredsOptions) resolveForCRSecrets(ctx context.Context, rc *resolvedCluster) error {
	o.log.Info("Connecting to Hive cluster")
	hiveKubeClient, err := o.connectHive(rc.ocmConn, rc.internalID)
	if err != nil {
		return fmt.Errorf("failed to connect to hive: %w", err)
	}
	rc.hiveClient = hiveKubeClient

	o.log.Info("Resolving account claim from Hive")
	claimName, err := o.resolveAccountClaimName(ctx, hiveKubeClient, rc.ocmConn, rc.internalID)
	if err != nil {
		return fmt.Errorf("failed to resolve account claim: %w", err)
	}
	rc.claimName = claimName
	o.log.WithField("claim", claimName).Info("Account claim resolved")

	o.log.WithField("account", claimName).Info("Fetching Account CR")
	account, err := k8s.GetAWSAccount(ctx, hiveKubeClient, common.AWSAccountNamespace, claimName)
	if err != nil {
		return fmt.Errorf("failed to get Account CR '%s': %w", claimName, err)
	}
	if account.Spec.ManualSTSMode {
		return fmt.Errorf("account %s is STS — no IAM user credentials to manage", claimName)
	}
	rc.account = account
	rc.accountID = account.Spec.AwsAccountID

	o.log.Info("Connecting to managed cluster via backplane")
	managedClient, err := o.connectManagedCluster(rc.internalID)
	if err != nil {
		return fmt.Errorf("managed cluster connection required for --cr-secrets: %w", err)
	}
	rc.managedClient = managedClient

	return nil
}

// buildAWSClientViaEnv creates an AWS client from AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY,
// and AWS_SESSION_TOKEN environment variables (e.g. set by rh-aws-saml-login --output env).
// Reads env vars directly and feeds them to roleChainToSupport, bypassing NewAwsClient
// which applies proxy config that interferes with credential chain resolution.
func (o *awsCredsOptions) buildAWSClientViaEnv(ctx context.Context, hiveClient client.Client, account *awsv1alpha1.Account, accountID, suffixLabel string) (awsprovider.Client, error) {
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")

	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("--aws-use-env requires AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables (e.g. from rh-aws-saml-login --output env)")
	}

	awsSetupClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.ClientInput{
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
		SessionToken:    sessionToken,
		Region:          "us-east-1",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS client from environment credentials: %w", err)
	}

	return o.roleChainToSupport(ctx, awsSetupClient, hiveClient, account, accountID, suffixLabel)
}

// buildAWSClientViaDefaultChain uses the default AWS credential chain (env vars, ~/.aws/config)
// as a fallback when backplane is unavailable.
func (o *awsCredsOptions) buildAWSClientViaDefaultChain(ctx context.Context, hiveClient client.Client, account *awsv1alpha1.Account, accountID, suffixLabel string) (awsprovider.Client, error) {
	o.log.Info("Using default AWS credential chain (env vars / ~/.aws/config)")
	awsSetupClient, err := awsprovider.NewAwsClient("", "us-east-1", "")
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS client from default credential chain: %w", err)
	}
	return o.roleChainToSupport(ctx, awsSetupClient, hiveClient, account, accountID, suffixLabel)
}

// connectManagedCluster establishes a k8s client to the managed cluster via backplane for CR secret operations.
func (o *awsCredsOptions) connectManagedCluster(clusterID string) (client.Client, error) {
	managedScheme := runtime.NewScheme()
	if err := corev1.AddToScheme(managedScheme); err != nil {
		return nil, fmt.Errorf("failed to register corev1 scheme: %w", err)
	}
	if err := ccov1.AddToScheme(managedScheme); err != nil {
		return nil, fmt.Errorf("failed to register cloudcredential scheme: %w", err)
	}

	managedClient, err := k8s.NewAsBackplaneClusterAdmin(
		clusterID,
		client.Options{Scheme: managedScheme},
		o.reason,
		fmt.Sprintf("Elevation required for aws-creds on %s", clusterID),
	)
	if err != nil {
		return nil, err
	}
	return managedClient, nil
}
