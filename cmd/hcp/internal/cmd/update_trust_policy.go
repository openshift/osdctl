package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
)

type UpdateTrustPolicyRequest struct {
	ClusterID     string
	SourceMC      string
	ReplacementMC string
	AWSProfile    string
}

// TrustPolicyUpdater prepares and applies backup-role trust policy updates.
type TrustPolicyUpdater interface {
	PrepareTrustPolicyUpdate(ctx context.Context, request UpdateTrustPolicyRequest) (TrustPolicyUpdatePlan, error)
	UpdateTrustPolicy(ctx context.Context, plan TrustPolicyUpdatePlan) error
}

// TrustPolicyUpdatePlan describes a trust policy update ready for confirmation.
type TrustPolicyUpdatePlan struct {
	RoleName                       string
	SourceManagementCluster        string
	SourceManagementClusterID      string
	ReplacementManagementCluster   string
	ReplacementManagementClusterID string

	client         awsprovider.Client
	policyDocument string
}

// DefaultTrustPolicyUpdater is the production trust-policy update implementation.
type DefaultTrustPolicyUpdater struct {
	ocmClient trustPolicyOCMClient
	awsClient internalclient.AWSAccountClientProvider
}

type DefaultTrustPolicyUpdaterConfig struct {
	AWSClient internalclient.AWSAccountClientProvider
}

func (c *DefaultTrustPolicyUpdaterConfig) Options(opts ...DefaultTrustPolicyUpdaterOption) {
	for _, opt := range opts {
		opt.ConfigureDefaultTrustPolicyUpdater(c)
	}
}

func (c *DefaultTrustPolicyUpdaterConfig) Defaults() {
	if c.AWSClient == nil {
		c.AWSClient = internalclient.NewAWSAccountClientProvider()
	}
}

type DefaultTrustPolicyUpdaterOption interface {
	ConfigureDefaultTrustPolicyUpdater(*DefaultTrustPolicyUpdaterConfig)
}

type trustPolicyOCMClient interface {
	internalclient.ClusterResolver
	Cluster(identifier string) (*cmv1.Cluster, error)
	BackupConfig(ctx context.Context, clusterID, managementCluster string) (*cmv1.AWSBackupConfig, error)
}

func NewDefaultTrustPolicyUpdater(ocmClient trustPolicyOCMClient, opts ...DefaultTrustPolicyUpdaterOption) *DefaultTrustPolicyUpdater {
	var cfg DefaultTrustPolicyUpdaterConfig
	cfg.Options(opts...)
	cfg.Defaults()

	return &DefaultTrustPolicyUpdater{ocmClient: ocmClient, awsClient: cfg.AWSClient}
}

func (r *DefaultTrustPolicyUpdater) PrepareTrustPolicyUpdate(ctx context.Context, request UpdateTrustPolicyRequest) (TrustPolicyUpdatePlan, error) {
	cluster, err := r.ocmClient.Resolve(ctx, request.ClusterID)
	if err != nil {
		return TrustPolicyUpdatePlan{}, fmt.Errorf("resolving cluster: %w", err)
	}
	if cluster.HCPClusterRegion == "" {
		return TrustPolicyUpdatePlan{}, fmt.Errorf("HCP cluster %s has no AWS region", cluster.HCPClusterID)
	}

	sourceMCName := cluster.MgmtClusterName
	sourceMCID := cluster.MgmtClusterID
	if request.SourceMC != "" {
		source, err := r.ocmClient.Cluster(request.SourceMC)
		if err != nil {
			return TrustPolicyUpdatePlan{}, fmt.Errorf("resolving source management cluster %s: %w", request.SourceMC, err)
		}
		sourceMCName = source.Name()
		sourceMCID = source.ID()
	}
	if sourceMCName == "" || sourceMCID == "" {
		return TrustPolicyUpdatePlan{}, fmt.Errorf("source management cluster is incomplete")
	}

	replacement, err := r.ocmClient.Cluster(request.ReplacementMC)
	if err != nil {
		return TrustPolicyUpdatePlan{}, fmt.Errorf("resolving replacement management cluster %s: %w", request.ReplacementMC, err)
	}
	if replacement.ID() == "" {
		return TrustPolicyUpdatePlan{}, fmt.Errorf("replacement management cluster %s has no ID", request.ReplacementMC)
	}

	sourceBackupConfig, err := r.ocmClient.BackupConfig(ctx, cluster.HCPClusterID, sourceMCName)
	if err != nil {
		return TrustPolicyUpdatePlan{}, err
	}
	replacementBackupConfig, err := r.ocmClient.BackupConfig(ctx, cluster.HCPClusterID, replacement.Name())
	if err != nil {
		return TrustPolicyUpdatePlan{}, fmt.Errorf("verifying replacement management cluster backup configuration: %w", err)
	}
	if replacementBackupConfig.AccountId() != sourceBackupConfig.AccountId() {
		return TrustPolicyUpdatePlan{}, fmt.Errorf("replacement management cluster %s uses backup account %s, expected %s", replacement.Name(), replacementBackupConfig.AccountId(), sourceBackupConfig.AccountId())
	}

	backupClient, err := r.awsClient.AccountClient(request.AWSProfile, cluster.HCPClusterRegion, sourceBackupConfig.AccountId())
	if err != nil {
		return TrustPolicyUpdatePlan{}, err
	}

	roleName := fmt.Sprintf("rosa-hcp-bkp-%s-%s", sourceMCName, cluster.HCPClusterID)
	updated, err := updatedTrustPolicy(backupClient, roleName, sourceMCID, replacement.ID())
	if err != nil {
		return TrustPolicyUpdatePlan{}, err
	}
	if updated == "" {
		return TrustPolicyUpdatePlan{}, nil
	}

	return TrustPolicyUpdatePlan{RoleName: roleName, SourceManagementCluster: sourceMCName, SourceManagementClusterID: sourceMCID, ReplacementManagementCluster: replacement.Name(), ReplacementManagementClusterID: replacement.ID(), client: backupClient, policyDocument: updated}, nil
}

func (r *DefaultTrustPolicyUpdater) UpdateTrustPolicy(ctx context.Context, plan TrustPolicyUpdatePlan) error {
	if plan.policyDocument == "" {
		return nil
	}
	if _, err := plan.client.UpdateAssumeRolePolicy(&iam.UpdateAssumeRolePolicyInput{RoleName: awssdk.String(plan.RoleName), PolicyDocument: awssdk.String(plan.policyDocument)}); err != nil {
		return fmt.Errorf("updating trust policy for role %s: %w", plan.RoleName, err)
	}
	return nil
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
