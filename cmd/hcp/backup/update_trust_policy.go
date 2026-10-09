package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/osdCloud"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type updateTrustPolicyFlags struct {
	clusterID     string
	sourceMC      string
	replacementMC string
	awsProfile    string
}

type trustPolicyUpdateRunner struct {
	resolver            ClusterResolver
	getCluster          func(string) (*cmv1.Cluster, error)
	getBackupConfig     func(context.Context, string, string) (*cmv1.AWSBackupConfig, error)
	newAWSClient        func(string, string, string) (awsprovider.Client, error)
	newAWSClientInput   func(*awsprovider.ClientInput) (awsprovider.Client, error)
	generateSessionName func(awsprovider.Client) (string, error)
	getAWSPartition     func(awsprovider.Client) (string, error)
	generateCredentials func(awsprovider.Client, string, string, string) (*ststypes.Credentials, error)
	confirm             func(io.Reader) bool
	in                  io.Reader
	promptWriter        io.Writer
}

func newCmdUpdateTrustPolicy() *cobra.Command {
	flags := &updateTrustPolicyFlags{}
	cmd := &cobra.Command{
		Use:   "update-trust-policy --cluster-id <cluster-id> --replacement-mc <management-cluster>",
		Short: "Allow a replacement management cluster to access HCP backups",
		Example: "  osdctl hcp backup update-trust-policy --cluster-id ${CLUSTER_ID} --replacement-mc ${REPLACEMENT_MC}\n" +
			"  osdctl hcp backup update-trust-policy --cluster-id ${CLUSTER_ID} --source-mc ${SOURCE_MC} --replacement-mc ${REPLACEMENT_MC} --profile ${AWS_PROFILE}",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := logrus.New()
			logger.SetOutput(cmd.ErrOrStderr())

			ocmConn, err := utils.CreateConnection()
			if err != nil {
				return fmt.Errorf("creating OCM connection: %w", err)
			}
			defer ocmConn.Close()

			runner := newTrustPolicyUpdateRunner(ocmConn, logger, cmd.InOrStdin(), cmd.ErrOrStderr())
			return runner.run(cmd.Context(), *flags)
		},
	}

	cmd.Flags().StringVarP(&flags.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().StringVar(&flags.sourceMC, "source-mc", "", "Internal ID, name, or external ID of the management cluster currently trusted by the backup role")
	cmd.Flags().StringVar(&flags.replacementMC, "replacement-mc", "", "Internal ID, name, or external ID of the replacement management cluster")
	cmd.Flags().StringVarP(&flags.awsProfile, "profile", "p", "", "AWS profile used to assume the DR account role")
	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("replacement-mc")

	return cmd
}

func newTrustPolicyUpdateRunner(ocmConn *ocmsdk.Connection, logger *logrus.Logger, in io.Reader, promptWriter io.Writer) *trustPolicyUpdateRunner {
	runner := &trustPolicyUpdateRunner{
		resolver: &ocmClusterResolver{ocmConn: ocmConn, logger: logger},
		getCluster: func(identifier string) (*cmv1.Cluster, error) {
			return utils.GetClusterAnyStatus(ocmConn, identifier)
		},
		newAWSClient:        awsprovider.NewAwsClient,
		newAWSClientInput:   awsprovider.NewAwsClientWithInput,
		generateSessionName: osdCloud.GenerateRoleSessionName,
		getAWSPartition:     awsprovider.GetAwsPartition,
		generateCredentials: osdCloud.GenerateOrganizationAccountAccessCredentials,
		confirm:             confirmTrustPolicyUpdate,
		in:                  in,
		promptWriter:        promptWriter,
	}
	runner.getBackupConfig = func(ctx context.Context, clusterID, managementCluster string) (*cmv1.AWSBackupConfig, error) {
		return getBackupConfig(ctx, ocmConn, clusterID, managementCluster)
	}
	return runner
}

func (r *trustPolicyUpdateRunner) run(ctx context.Context, flags updateTrustPolicyFlags) error {
	cluster, err := r.resolver.Resolve(ctx, flags.clusterID)
	if err != nil {
		return fmt.Errorf("resolving cluster: %w", err)
	}
	if cluster.HCPClusterRegion == "" {
		return fmt.Errorf("HCP cluster %s has no AWS region", cluster.HCPClusterID)
	}

	sourceMCName := cluster.MgmtClusterName
	sourceMCID := cluster.MgmtClusterID
	if flags.sourceMC != "" {
		source, err := r.getCluster(flags.sourceMC)
		if err != nil {
			return fmt.Errorf("resolving source management cluster %s: %w", flags.sourceMC, err)
		}
		sourceMCName = source.Name()
		sourceMCID = source.ID()
	}
	if sourceMCName == "" || sourceMCID == "" {
		return fmt.Errorf("source management cluster is incomplete")
	}

	replacement, err := r.getCluster(flags.replacementMC)
	if err != nil {
		return fmt.Errorf("resolving replacement management cluster %s: %w", flags.replacementMC, err)
	}
	if replacement.ID() == "" {
		return fmt.Errorf("replacement management cluster %s has no ID", flags.replacementMC)
	}

	sourceBackupConfig, err := r.getBackupConfig(ctx, cluster.HCPClusterID, sourceMCName)
	if err != nil {
		return err
	}
	replacementBackupConfig, err := r.getBackupConfig(ctx, cluster.HCPClusterID, replacement.Name())
	if err != nil {
		return fmt.Errorf("verifying replacement management cluster backup configuration: %w", err)
	}
	if replacementBackupConfig.AccountId() != sourceBackupConfig.AccountId() {
		return fmt.Errorf("replacement management cluster %s uses backup account %s, expected %s", replacement.Name(), replacementBackupConfig.AccountId(), sourceBackupConfig.AccountId())
	}

	backupClient, err := r.backupClient(flags.awsProfile, cluster.HCPClusterRegion, sourceBackupConfig.AccountId())
	if err != nil {
		return err
	}

	roleName := fmt.Sprintf("rosa-hcp-bkp-%s-%s", sourceMCName, cluster.HCPClusterID)
	updated, err := updatedTrustPolicy(backupClient, roleName, sourceMCID, replacement.ID())
	if err != nil {
		return err
	}
	if updated == "" {
		return nil
	}

	fmt.Fprintf(r.promptWriter, "Update backup role trust policy?\nRole: %s\nSource management cluster: %s (%s)\nReplacement management cluster: %s (%s)\nContinue? [y/N] ", roleName, sourceMCName, sourceMCID, replacement.Name(), replacement.ID())
	if !r.confirm(r.in) {
		fmt.Fprintln(r.promptWriter, "Backup role trust policy update cancelled.")
		return nil
	}

	if _, err := backupClient.UpdateAssumeRolePolicy(&iam.UpdateAssumeRolePolicyInput{RoleName: awssdk.String(roleName), PolicyDocument: awssdk.String(updated)}); err != nil {
		return fmt.Errorf("updating trust policy for role %s: %w", roleName, err)
	}
	return nil
}

func (r *trustPolicyUpdateRunner) backupClient(profile, region, accountID string) (awsprovider.Client, error) {
	baseClient, err := r.newAWSClient(profile, region, "")
	if err != nil {
		return nil, fmt.Errorf("creating AWS client: %w", err)
	}
	sessionName, err := r.generateSessionName(baseClient)
	if err != nil {
		return nil, fmt.Errorf("generating AWS role session name: %w", err)
	}
	partition, err := r.getAWSPartition(baseClient)
	if err != nil {
		return nil, fmt.Errorf("getting AWS partition: %w", err)
	}
	credentials, err := r.generateCredentials(baseClient, accountID, sessionName, partition)
	if err != nil {
		return nil, fmt.Errorf("assuming DR account role: %w", err)
	}
	if credentials == nil || credentials.AccessKeyId == nil || credentials.SecretAccessKey == nil || credentials.SessionToken == nil {
		return nil, fmt.Errorf("assuming DR account role returned incomplete credentials")
	}
	client, err := r.newAWSClientInput(&awsprovider.ClientInput{AccessKeyID: *credentials.AccessKeyId, SecretAccessKey: *credentials.SecretAccessKey, SessionToken: *credentials.SessionToken, Region: region})
	if err != nil {
		return nil, fmt.Errorf("creating backup IAM client: %w", err)
	}
	return client, nil
}

func updatedTrustPolicy(client awsprovider.Client, roleName, sourceMCID, replacementMCID string) (string, error) {
	role, err := client.GetRole(&iam.GetRoleInput{RoleName: awssdk.String(roleName)})
	if err != nil {
		return "", fmt.Errorf("getting role %s: %w", roleName, err)
	}
	if role == nil || role.Role == nil || role.Role.AssumeRolePolicyDocument == nil {
		return "", fmt.Errorf("role %s has no trust policy", roleName)
	}
	document, err := url.PathUnescape(*role.Role.AssumeRolePolicyDocument)
	if err != nil {
		return "", fmt.Errorf("decoding trust policy for role %s: %w", roleName, err)
	}

	var policy map[string]any
	if err := json.Unmarshal([]byte(document), &policy); err != nil {
		return "", fmt.Errorf("parsing trust policy for role %s: %w", roleName, err)
	}
	changed, err := replaceTrustPolicyManagementCluster(policy, sourceMCID, replacementMCID)
	if err != nil {
		return "", err
	}
	if !changed {
		return "", nil
	}
	updated, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("encoding trust policy for role %s: %w", roleName, err)
	}
	return string(updated), nil
}

func replaceTrustPolicyManagementCluster(policy map[string]any, sourceMCID, replacementMCID string) (bool, error) {
	statements, ok := policy["Statement"].([]any)
	if !ok {
		return false, fmt.Errorf("trust policy has no statements")
	}
	matched := false
	changed := false
	for _, item := range statements {
		statement, ok := item.(map[string]any)
		if !ok || !hasWebIdentityAction(statement["Action"]) {
			continue
		}
		principal, ok := statement["Principal"].(map[string]any)
		if !ok {
			continue
		}
		var updatedFederated any
		federatedChanged := false
		switch federated := principal["Federated"].(type) {
		case string:
			updated, sourceMatches := replaceFinalPathSegment(federated, sourceMCID, replacementMCID)
			if !sourceMatches {
				continue
			}
			updatedFederated = updated
			federatedChanged = updated != federated
		case []any:
			updated := append([]any(nil), federated...)
			sourceMatches := false
			for i, item := range federated {
				provider, ok := item.(string)
				if !ok {
					continue
				}
				replacement, matches := replaceFinalPathSegment(provider, sourceMCID, replacementMCID)
				if !matches {
					continue
				}
				updated[i] = replacement
				sourceMatches = true
				federatedChanged = federatedChanged || replacement != provider
			}
			if !sourceMatches {
				continue
			}
			updatedFederated = updated
		default:
			continue
		}
		condition, ok := statement["Condition"].(map[string]any)
		if !ok {
			return false, fmt.Errorf("web identity trust statement has no condition")
		}
		stringEquals, ok := condition["StringEquals"].(map[string]any)
		if !ok {
			return false, fmt.Errorf("web identity trust statement has no StringEquals condition")
		}
		var subjectKey, updatedSubjectKey string
		subjectConditionFound := false
		for key := range stringEquals {
			updatedKey, sourceMatches := replaceSubjectKeyManagementCluster(key, sourceMCID, replacementMCID)
			if !sourceMatches {
				continue
			}
			subjectConditionFound = true
			if updatedKey != key {
				if _, exists := stringEquals[updatedKey]; exists {
					return false, fmt.Errorf("web identity trust statement has a conflicting subject condition")
				}
				subjectKey = key
				updatedSubjectKey = updatedKey
			}
			break
		}
		if !subjectConditionFound {
			return false, fmt.Errorf("web identity trust statement has no subject condition")
		}
		matched = true
		principal["Federated"] = updatedFederated
		changed = changed || federatedChanged
		if subjectKey != "" {
			value := stringEquals[subjectKey]
			delete(stringEquals, subjectKey)
			stringEquals[updatedSubjectKey] = value
			changed = true
		}
	}
	if !matched {
		return false, fmt.Errorf("trust policy has no web identity trust statement for source management cluster")
	}
	return changed, nil
}

func hasWebIdentityAction(value any) bool {
	switch action := value.(type) {
	case string:
		return action == "sts:AssumeRoleWithWebIdentity"
	case []any:
		for _, item := range action {
			if item, ok := item.(string); ok && item == "sts:AssumeRoleWithWebIdentity" {
				return true
			}
		}
	}
	return false
}

func replaceFinalPathSegment(value, source, replacement string) (string, bool) {
	lastSlash := strings.LastIndex(value, "/")
	if lastSlash < 0 || lastSlash == len(value)-1 || value[lastSlash+1:] != source {
		return value, false
	}
	return value[:lastSlash+1] + replacement, true
}

func replaceSubjectKeyManagementCluster(key, source, replacement string) (string, bool) {
	const suffix = ":sub"
	if !strings.HasSuffix(key, suffix) {
		return "", false
	}
	prefix := strings.TrimSuffix(key, suffix)
	lastSlash := strings.LastIndex(prefix, "/")
	if lastSlash < 0 || lastSlash == len(prefix)-1 || prefix[lastSlash+1:] != source {
		return key, false
	}
	return prefix[:lastSlash+1] + replacement + suffix, true
}

func confirmTrustPolicyUpdate(in io.Reader) bool {
	var response string
	if _, err := fmt.Fscanln(in, &response); err != nil {
		return false
	}
	return strings.EqualFold(response, "y") || strings.EqualFold(response, "yes")
}
