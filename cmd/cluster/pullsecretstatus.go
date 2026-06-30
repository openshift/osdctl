package cluster

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/controller"
	"github.com/openshift/osdctl/pkg/utils"
)

// nolint:gosec
const pullSecStatusUsageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}

Required Flags (one of --cluster-id or --account-id):
  -C, --cluster-id string   Any cluster owned by the account (used to resolve the owner)
  -A, --account-id string   OCM account ID directly
      --reason string        Elevation reason for cluster connections

Optional Flags:
      --validate             Validate all clusters' pull secrets against OCM
`

type pullSecretAuditOptions struct {
	clusterID string
	accountID string
	reason    string
	validate  bool
	logger    *logrus.Logger

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

func newCmdPullSecretAudit(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := &pullSecretAuditOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
		logger:        newAuditLogger(),
	}
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit pull secret status for all clusters owned by an account",
		Long: `Audit pull secret status for all clusters sharing the same OCM account.

Given a cluster ID or account ID, resolves the owner account and lists all
clusters owned by that account. Compares cluster creation dates against the
account's registry credential update timestamps to flag clusters that may
have stale pull secrets.

Use --validate to connect to each cluster and compare its pull secret
against the OCM access token and registry credential auths.

For validating a single cluster, use 'osdctl cluster pull-secret validate'.`,
		Example: `  # Overview of all clusters for the account
  osdctl cluster pull-secret audit -C 1kfmyclusterid --reason "OHSS-1234"

  # Using account ID directly
  osdctl cluster pull-secret audit -A 2g9OLHPkwDDcXvq2mt7kjfIQ0gf --reason "OHSS-1234"

  # Validate all clusters' pull secrets against OCM
  osdctl cluster pull-secret audit -C 1kfmyclusterid --reason "OHSS-1234" --validate`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		SilenceUsage:      true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			clusterSet := cmd.Flags().Changed("cluster-id")
			accountSet := cmd.Flags().Changed("account-id")
			if !clusterSet && !accountSet {
				return fmt.Errorf("one of --cluster-id or --account-id is required")
			}
			if clusterSet && accountSet {
				return fmt.Errorf("--cluster-id and --account-id are mutually exclusive")
			}
			idPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
			if ops.accountID != "" && !idPattern.MatchString(ops.accountID) {
				return fmt.Errorf("--account-id contains invalid characters")
			}
			if ops.clusterID != "" && !idPattern.MatchString(ops.clusterID) {
				return fmt.Errorf("--cluster-id contains invalid characters")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.run(cmd.Context())
		},
	}

	cmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Any cluster owned by the account (used to resolve the owner)")
	cmd.Flags().StringVarP(&ops.accountID, "account-id", "A", "", "OCM account ID directly")
	cmd.Flags().StringVar(&ops.reason, "reason", "", "Elevation reason for cluster connections")
	cmd.Flags().BoolVar(&ops.validate, "validate", false, "Validate all clusters' pull secrets against OCM")

	if err := cmd.MarkFlagRequired("reason"); err != nil {
		panic(fmt.Sprintf("failed to mark 'reason' as required: %v", err))
	}

	cmd.SetUsageTemplate(pullSecStatusUsageTemplate)

	return cmd
}

func newAuditLogger() *logrus.Logger {
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

func (o *pullSecretAuditOptions) run(ctx context.Context) error {
	out := o.Out
	logger := o.logger

	log.SetLogger(zap.New(zap.WriteTo(o.ErrOut), zap.Level(zapcore.WarnLevel)))

	logger.Info("Creating OCM connection")
	ocm, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("failed to create OCM client: %w", err)
	}
	defer func() {
		if closeErr := ocm.Close(); closeErr != nil {
			logger.Warnf("Cannot close the OCM connection: %v", closeErr)
		}
	}()

	// Resolve account — from cluster ID or directly
	var ownerAccountID, ownerUsername, ownerEmail string

	if o.accountID != "" {
		ownerAccount, err := utils.GetAccount(ocm, o.accountID)
		if err != nil {
			return fmt.Errorf("failed to get account %s: %w", o.accountID, err)
		}
		ownerAccountID = ownerAccount.ID()
		ownerUsername = ownerAccount.Username()
		ownerEmail = ownerAccount.Email()
		logger.Infof("Account resolved: %s (%s)", ownerUsername, ownerAccountID)
	} else {
		cluster, err := utils.GetClusterAnyStatus(ocm, o.clusterID)
		if err != nil {
			return fmt.Errorf("failed to get cluster: %w", err)
		}
		logger.Infof("Cluster resolved: %s (%s)", cluster.Name(), cluster.ID())

		subscription, err := utils.GetSubscription(ocm, cluster.ID())
		if err != nil {
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		ownerAccount, err := utils.GetAccount(ocm, subscription.Creator().ID())
		if err != nil {
			return fmt.Errorf("failed to get owner account: %w", err)
		}
		ownerAccountID = ownerAccount.ID()
		ownerUsername = ownerAccount.Username()
		ownerEmail = ownerAccount.Email()
		logger.Infof("Owner resolved: %s (account: %s)", ownerUsername, ownerAccountID)
	}

	logger.Info("Fetching registry credentials from OCM")
	latestCredUpdate, err := controller.GetLatestCredentialUpdate(ocm, ownerAccountID)
	if err != nil {
		logger.Warnf("Could not fetch registry credentials: %v", err)
	}

	logger.Info("Querying clusters for this account")
	clusters, err := controller.ListOwnerSubscriptions(ocm, ownerAccountID)
	if err != nil {
		return fmt.Errorf("failed to list subscriptions: %w", err)
	}

	// Fetch OCM data and collect validation results if --validate
	type checkResult struct {
		accessTokenResult *controller.PullSecretVerifyResult
		regCredResult     *controller.PullSecretVerifyResult
		err               error
	}
	checkResults := make(map[string]*checkResult)

	var auths map[string]*amv1.AccessTokenAuth
	hasAccessToken := false
	hasRegCreds := false

	if o.validate {
		logger.Infof("Fetching access token from OCM for owner '%s'", ownerUsername)
		_, auths, err = controller.FetchOwnerAccessToken(ocm, ownerUsername, logger)
		if err != nil {
			logger.Warnf("Could not fetch access token: %v", err)
			fmt.Fprintf(out, "\n%s Could not fetch OCM access token (may require region-lead permissions).\n", colorWarn("[WARN]"))
			fmt.Fprint(out, "Continue with registry credentials only? ")
			if !utils.ConfirmPrompt() {
				o.validate = false
			}
		} else {
			hasAccessToken = true
			logger.Infof("Retrieved %d auth entries from OCM access token", len(auths))
		}

		if o.validate {
			logger.Info("Fetching registry credentials from OCM")
			testCreds, regErr := utils.GetRegistryCredentials(ocm, ownerAccountID)
			if regErr != nil || len(testCreds) == 0 {
				logger.Warnf("Could not fetch registry credentials: %v", regErr)
			} else {
				hasRegCreds = true
				logger.Infof("Retrieved %d registry credentials from OCM", len(testCreds))
			}
		}

		if !hasAccessToken && !hasRegCreds {
			logger.Warn("Neither access token nor registry credentials available — skipping validation")
			fmt.Fprintf(out, "%s Cannot compare cluster pull secrets without OCM data. Skipping --validate.\n", colorWarn("[WARN]"))
			o.validate = false
		}
	}

	// Connect to clusters and collect results
	if o.validate {
		elevationReasons := []string{
			o.reason,
			"Checking pull secret status using osdctl pull-secret audit",
		}

		for _, c := range clusters {
			cr := &checkResult{}
			logger.Infof("Connecting to cluster %s (%s)", c.Name, c.ID)
			_, _, clientset, connErr := common.GetKubeConfigAndClient(c.ID, elevationReasons...)
			if connErr != nil {
				cr.err = fmt.Errorf("failed to connect: %v", connErr)
				checkResults[c.ID] = cr
				continue
			}

			if hasAccessToken {
				result, verifyErr := controller.CompareAccessTokenAuthsToCluster(ctx, clientset, auths, nil)
				if verifyErr != nil {
					logger.Warnf("Access token verification failed for %s: %v", c.ID, verifyErr)
				} else {
					cr.accessTokenResult = result
				}
			}

			if hasRegCreds {
				result, verifyErr := controller.CompareRegistryCredentialAuthsToCluster(ctx, ocm, clientset, ownerAccountID, ownerEmail, nil)
				if verifyErr != nil {
					logger.Warnf("Registry credential verification failed for %s: %v", c.ID, verifyErr)
				} else {
					cr.regCredResult = result
				}
			}

			checkResults[c.ID] = cr
		}
	}

	// --- Render ---

	fmt.Fprintf(out, "\n============================================================\n")
	fmt.Fprintf(out, " Owner:    %s (account: %s)\n", ownerUsername, ownerAccountID)
	fmt.Fprintf(out, " Email:    %s\n", ownerEmail)
	if !latestCredUpdate.IsZero() {
		fmt.Fprintf(out, " Registry credentials last updated: %s\n", latestCredUpdate.Format("2006-01-02 15:04:05 UTC"))
	}
	fmt.Fprintf(out, " Clusters: %d\n", len(clusters))
	fmt.Fprintf(out, "============================================================\n\n")

	staleCount := 0
	for i, c := range clusters {
		if i > 0 {
			fmt.Fprintln(out)
			fmt.Fprintln(out, "============================================================")
			fmt.Fprintln(out)
		}
		renderClusterBanner(out, c)

		cr, hasCheck := checkResults[c.ID]
		renderPSStatus(out, c, latestCredUpdate, &staleCount)

		if hasCheck && cr.err != nil {
			fmt.Fprintf(out, "  %s PS CHECK: %v\n", colorFail("[FAIL]"), cr.err)
		} else if hasCheck {
			if cr.accessTokenResult != nil {
				renderCheckTable(out, cr.accessTokenResult, c.ID, "ACCESS TOKEN AUTHS")
			} else if hasAccessToken {
				fmt.Fprintf(out, "  %s access token verification failed for this cluster\n", colorWarn("[WARN]"))
			}
			if cr.regCredResult != nil {
				renderCheckTable(out, cr.regCredResult, c.ID, "REGISTRY CREDENTIAL AUTHS")
			} else if hasRegCreds {
				fmt.Fprintf(out, "  %s registry credential verification failed for this cluster\n", colorWarn("[WARN]"))
			}
		}
	}

	validatedCount := 0
	for _, cr := range checkResults {
		if cr.err == nil {
			validatedCount++
		}
	}

	fmt.Fprintf(out, "\n%d cluster(s) found", len(clusters))
	if staleCount > 0 {
		fmt.Fprintf(out, ", %s %d potentially stale", colorWarn("[WARN]"), staleCount)
	}
	if o.validate {
		fmt.Fprintf(out, ", %d validated", validatedCount)
		if validatedCount < len(checkResults) {
			fmt.Fprintf(out, ", %d failed to connect", len(checkResults)-validatedCount)
		}
	}
	fmt.Fprintln(out, ".")

	if !o.validate {
		fmt.Fprintf(out, "Use --validate to check all clusters' pull secrets against OCM.\n")
	}

	return nil
}

func renderClusterBanner(out io.Writer, c controller.ClusterSummary) {
	label := color.New(color.FgBlue, color.Bold).SprintFunc()
	fmt.Fprintf(out, "%s  %s (%s)\n", label("Cluster:"), c.Name, c.ID)
	fmt.Fprintf(out, "%s  %s    %s  %s\n",
		label("Created:"), c.CreatedAt.Format("2006-01-02 15:04"),
		label("Status:"), c.Status)
}

func renderPSStatus(out io.Writer, c controller.ClusterSummary, latestCredUpdate time.Time, staleCount *int) {
	psLabel := color.New(color.FgCyan, color.Bold).SprintFunc()
	psDetail := color.New(color.FgCyan).SprintFunc()

	if latestCredUpdate.IsZero() {
		fmt.Fprintf(out, "  %s %s\n", psLabel("PS STATUS:"), psDetail("unknown — no credential timestamps available"))
	} else if c.CreatedAt.Before(latestCredUpdate) {
		fmt.Fprintf(out, "  %s %s %s\n", colorWarn("[WARN]"), psLabel("PS STATUS:"), psDetail("may be stale — created before last credential update"))
		*staleCount++
	} else {
		fmt.Fprintf(out, "  %s %s\n", psLabel("PS STATUS:"), psDetail("likely current — created after last credential update"))
	}
}

func renderCheckTable(out io.Writer, result *controller.PullSecretVerifyResult, clusterID string, sourceLabel string) {
	table := tablewriter.NewWriter(out)
	table.SetHeader([]string{sourceLabel, "TOKEN", "EMAIL", "STATUS"})
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetBorder(false)
	table.SetColumnSeparator("  ")
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlueColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlueColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlueColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlueColor},
	)

	mismatchStatus := color.New(color.FgYellow, color.Bold).SprintFunc()
	for _, ar := range result.AuthResults {
		status := colorOK("[OK]")
		tokenStr := "match"
		emailStr := "match"
		if !ar.OK {
			status = mismatchStatus("[!]")
			if ar.Detail == "not found in cluster secret" {
				tokenStr = "missing"
				emailStr = "missing"
			} else {
				if !ar.TokenMatch {
					tokenStr = "MISMATCH"
				}
				if !ar.EmailMatch {
					emailStr = "MISMATCH"
				}
			}
		}
		table.Append([]string{ar.Registry, tokenStr, emailStr, status})
		if !ar.OK && ar.Detail != "" && ar.Detail != "not found in cluster secret" {
			mismatchDetail := color.New(color.FgYellow).SprintFunc()
			table.Append([]string{"", mismatchDetail(ar.Detail), "", ""})
		}
	}
	table.Render()

	if result.Matched < result.Total {
		fmt.Fprintf(out, "  %s Verified %d/%d — consider 'osdctl cluster pull-secret update -C %s'\n",
			colorWarn("[WARN]"), result.Matched, result.Total, clusterID)
	}
	if len(result.MissingRequired) > 0 {
		fmt.Fprintf(out, "  %s missing required registries: %v\n",
			colorWarn("[WARN]"), result.MissingRequired)
	}
}
