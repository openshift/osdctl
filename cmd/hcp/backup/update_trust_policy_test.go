package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	awsmock "github.com/openshift/osdctl/pkg/provider/aws/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestReplaceTrustPolicyManagementCluster(t *testing.T) {
	t.Parallel()

	policy := map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{
			map[string]any{
				"Effect":    "Allow",
				"Action":    "sts:AssumeRoleWithWebIdentity",
				"Principal": map[string]any{"Federated": "arn:aws:iam::123456789012:oidc-provider/rh-sre-oidc.s3.us-east-1.amazonaws.com/old-mc"},
				"Condition": map[string]any{"StringEquals": map[string]any{
					"rh-sre-oidc.s3.us-east-1.amazonaws.com/old-mc:sub": "system:serviceaccount:openshift-adp:velero",
					"unrelated": "value",
				}},
			},
			map[string]any{"Effect": "Allow", "Action": "sts:AssumeRole", "Principal": map[string]any{"AWS": "arn:aws:iam::1:root"}},
		},
	}

	changed, err := replaceTrustPolicyManagementCluster(policy, "old-mc", "new-mc")
	require.NoError(t, err)
	assert.True(t, changed)

	statement := policy["Statement"].([]any)[0].(map[string]any)
	assert.Equal(t, "arn:aws:iam::123456789012:oidc-provider/rh-sre-oidc.s3.us-east-1.amazonaws.com/new-mc", statement["Principal"].(map[string]any)["Federated"])
	conditions := statement["Condition"].(map[string]any)["StringEquals"].(map[string]any)
	assert.Equal(t, "system:serviceaccount:openshift-adp:velero", conditions["rh-sre-oidc.s3.us-east-1.amazonaws.com/new-mc:sub"])
	assert.Equal(t, "value", conditions["unrelated"])
	assert.Equal(t, "sts:AssumeRole", policy["Statement"].([]any)[1].(map[string]any)["Action"])
}

func TestReplaceTrustPolicyManagementClusterNoOp(t *testing.T) {
	t.Parallel()

	policy := map[string]any{"Statement": []any{map[string]any{
		"Action":    "sts:AssumeRoleWithWebIdentity",
		"Principal": map[string]any{"Federated": "arn:aws:iam::1:oidc-provider/issuer/new-mc"},
		"Condition": map[string]any{"StringEquals": map[string]any{"issuer/new-mc:sub": "subject"}},
	}}}

	changed, err := replaceTrustPolicyManagementCluster(policy, "new-mc", "new-mc")
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestReplaceTrustPolicyManagementClusterOnlyUpdatesSource(t *testing.T) {
	t.Parallel()

	policy := map[string]any{"Statement": []any{
		map[string]any{
			"Action":    []any{"sts:AssumeRole", "sts:AssumeRoleWithWebIdentity"},
			"Principal": map[string]any{"Federated": "arn:aws:iam::1:oidc-provider/issuer/source"},
			"Condition": map[string]any{"StringEquals": map[string]any{"issuer/source:sub": "source-subject"}},
		},
		map[string]any{
			"Action":    "sts:AssumeRoleWithWebIdentity",
			"Principal": map[string]any{"Federated": "arn:aws:iam::1:oidc-provider/issuer/other"},
			"Condition": map[string]any{"StringEquals": map[string]any{"issuer/other:sub": "other-subject"}},
		},
	}}

	changed, err := replaceTrustPolicyManagementCluster(policy, "source", "replacement")
	require.NoError(t, err)
	assert.True(t, changed)

	statements := policy["Statement"].([]any)
	source := statements[0].(map[string]any)
	assert.Equal(t, "arn:aws:iam::1:oidc-provider/issuer/replacement", source["Principal"].(map[string]any)["Federated"])
	assert.Equal(t, "source-subject", source["Condition"].(map[string]any)["StringEquals"].(map[string]any)["issuer/replacement:sub"])
	other := statements[1].(map[string]any)
	assert.Equal(t, "arn:aws:iam::1:oidc-provider/issuer/other", other["Principal"].(map[string]any)["Federated"])
	assert.Equal(t, "other-subject", other["Condition"].(map[string]any)["StringEquals"].(map[string]any)["issuer/other:sub"])
}

func TestReplaceTrustPolicyManagementClusterUpdatesFederatedArray(t *testing.T) {
	t.Parallel()

	policy := map[string]any{"Statement": []any{map[string]any{
		"Action": "sts:AssumeRoleWithWebIdentity",
		"Principal": map[string]any{"Federated": []any{
			"arn:aws:iam::1:oidc-provider/issuer/source",
			"arn:aws:iam::1:oidc-provider/issuer/other",
			42,
			"arn:aws:iam::1:oidc-provider/another/source",
		}},
		"Condition": map[string]any{"StringEquals": map[string]any{"issuer/source:sub": "subject"}},
	}}}

	changed, err := replaceTrustPolicyManagementCluster(policy, "source", "replacement")
	require.NoError(t, err)
	assert.True(t, changed)

	statement := policy["Statement"].([]any)[0].(map[string]any)
	assert.Equal(t, []any{
		"arn:aws:iam::1:oidc-provider/issuer/replacement",
		"arn:aws:iam::1:oidc-provider/issuer/other",
		42,
		"arn:aws:iam::1:oidc-provider/another/replacement",
	}, statement["Principal"].(map[string]any)["Federated"])
	assert.Equal(t, "subject", statement["Condition"].(map[string]any)["StringEquals"].(map[string]any)["issuer/replacement:sub"])
}

func TestReplaceTrustPolicyManagementClusterRejectsSubjectCollision(t *testing.T) {
	t.Parallel()

	policy := map[string]any{"Statement": []any{map[string]any{
		"Action":    "sts:AssumeRoleWithWebIdentity",
		"Principal": map[string]any{"Federated": "arn:aws:iam::1:oidc-provider/issuer/source"},
		"Condition": map[string]any{"StringEquals": map[string]any{
			"issuer/source:sub":      "source-subject",
			"issuer/replacement:sub": "replacement-subject",
		}},
	}}}

	_, err := replaceTrustPolicyManagementCluster(policy, "source", "replacement")
	assert.ErrorContains(t, err, "conflicting subject condition")
	statement := policy["Statement"].([]any)[0].(map[string]any)
	assert.Equal(t, "arn:aws:iam::1:oidc-provider/issuer/source", statement["Principal"].(map[string]any)["Federated"])
}

func TestReplaceTrustPolicyManagementClusterRejectsPolicyWithoutSource(t *testing.T) {
	t.Parallel()

	policy := map[string]any{"Statement": []any{map[string]any{
		"Action":    "sts:AssumeRoleWithWebIdentity",
		"Principal": map[string]any{"Federated": "arn:aws:iam::1:oidc-provider/issuer/other"},
		"Condition": map[string]any{"StringEquals": map[string]any{"issuer/other:sub": "other-subject"}},
	}}}

	_, err := replaceTrustPolicyManagementCluster(policy, "source", "replacement")
	assert.ErrorContains(t, err, "source management cluster")
}

func TestReplaceTrustPolicyManagementClusterRejectsInvalidPolicies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy map[string]any
		want   string
	}{
		{name: "no statements", policy: map[string]any{}, want: "no statements"},
		{name: "no web identity statement", policy: map[string]any{"Statement": []any{map[string]any{"Action": "sts:AssumeRole"}}}, want: "no web identity"},
		{name: "missing condition", policy: map[string]any{"Statement": []any{map[string]any{"Action": "sts:AssumeRoleWithWebIdentity", "Principal": map[string]any{"Federated": "provider/old"}}}}, want: "no condition"},
		{name: "missing subject", policy: map[string]any{"Statement": []any{map[string]any{"Action": "sts:AssumeRoleWithWebIdentity", "Principal": map[string]any{"Federated": "provider/old"}, "Condition": map[string]any{"StringEquals": map[string]any{"issuer/aud": "audience"}}}}}, want: "no subject"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := replaceTrustPolicyManagementCluster(tt.policy, "old", "new")
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

func TestUpdatedTrustPolicy(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := awsmock.NewMockClient(ctrl)
	document := `{"Statement":[{"Action":"sts:AssumeRoleWithWebIdentity","Principal":{"Federated":"arn:aws:iam::1:oidc-provider/issuer/old"},"Condition":{"StringEquals":{"issuer/old:sub":"subject"}}}]}`
	client.EXPECT().GetRole(gomock.Cond(func(input *iam.GetRoleInput) bool {
		return awssdk.ToString(input.RoleName) == "role"
	})).Return(&iam.GetRoleOutput{Role: &iamtypes.Role{AssumeRolePolicyDocument: awssdk.String(url.PathEscape(document))}}, nil)

	updated, err := updatedTrustPolicy(client, "role", "old", "new")
	require.NoError(t, err)
	assert.JSONEq(t, `{"Statement":[{"Action":"sts:AssumeRoleWithWebIdentity","Principal":{"Federated":"arn:aws:iam::1:oidc-provider/issuer/new"},"Condition":{"StringEquals":{"issuer/new:sub":"subject"}}}]}`, updated)
}

func TestTrustPolicyUpdateRunnerUpdatesAfterConfirmation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	baseClient := awsmock.NewMockClient(ctrl)
	backupClient := awsmock.NewMockClient(ctrl)
	document := `{"Statement":[{"Action":"sts:AssumeRoleWithWebIdentity","Principal":{"Federated":"arn:aws:iam::1:oidc-provider/issuer/source-id"},"Condition":{"StringEquals":{"issuer/source-id:sub":"subject"}}}]}`
	backupClient.EXPECT().GetRole(gomock.Any()).Return(&iam.GetRoleOutput{Role: &iamtypes.Role{AssumeRolePolicyDocument: awssdk.String(document)}}, nil)
	backupClient.EXPECT().UpdateAssumeRolePolicy(gomock.Cond(func(input *iam.UpdateAssumeRolePolicyInput) bool {
		if awssdk.ToString(input.RoleName) != "rosa-hcp-bkp-source-name-hcp-id" {
			return false
		}
		var policy struct {
			Statement []struct {
				Principal struct {
					Federated string `json:"Federated"`
				} `json:"Principal"`
				Condition struct {
					StringEquals map[string]string `json:"StringEquals"`
				} `json:"Condition"`
			} `json:"Statement"`
		}
		if json.Unmarshal([]byte(awssdk.ToString(input.PolicyDocument)), &policy) != nil || len(policy.Statement) != 1 {
			return false
		}
		statement := policy.Statement[0]
		return strings.HasSuffix(statement.Principal.Federated, "/replacement-id") &&
			statement.Condition.StringEquals["issuer/replacement-id:sub"] == "subject"
	})).Return(&iam.UpdateAssumeRolePolicyOutput{}, nil)

	var prompt strings.Builder
	runner := &trustPolicyUpdateRunner{
		resolver: &staticClusterResolver{clusterInfo: ClusterInfo{HCPClusterID: "hcp-id", HCPClusterRegion: "us-east-1", MgmtClusterID: "source-id", MgmtClusterName: "source-name"}},
		getCluster: func(identifier string) (*cmv1.Cluster, error) {
			if identifier != "replacement" {
				return nil, fmt.Errorf("unexpected identifier %s", identifier)
			}
			return newTestCluster(t, "replacement-id", "replacement-name"), nil
		},
		getBackupConfig: func(_ context.Context, clusterID, managementCluster string) (*cmv1.AWSBackupConfig, error) {
			assert.Equal(t, "hcp-id", clusterID)
			switch managementCluster {
			case "source-name", "replacement-name":
				return backupConfig(t, managementCluster, "bucket", "123456789012"), nil
			default:
				return nil, fmt.Errorf("unexpected management cluster %s", managementCluster)
			}
		},
		newAWSClient: func(profile, region, configFile string) (awsprovider.Client, error) {
			assert.Empty(t, profile)
			assert.Equal(t, "us-east-1", region)
			return baseClient, nil
		},
		generateSessionName: func(client awsprovider.Client) (string, error) {
			assert.Same(t, baseClient, client)
			return "session", nil
		},
		getAWSPartition: func(client awsprovider.Client) (string, error) { assert.Same(t, baseClient, client); return "aws", nil },
		generateCredentials: func(client awsprovider.Client, accountID, session, partition string) (*ststypes.Credentials, error) {
			assert.Same(t, baseClient, client)
			assert.Equal(t, "123456789012", accountID)
			return &ststypes.Credentials{AccessKeyId: awssdk.String("key"), SecretAccessKey: awssdk.String("secret"), SessionToken: awssdk.String("token")}, nil
		},
		newAWSClientInput: func(input *awsprovider.ClientInput) (awsprovider.Client, error) {
			assert.Equal(t, &awsprovider.ClientInput{AccessKeyID: "key", SecretAccessKey: "secret", SessionToken: "token", Region: "us-east-1"}, input)
			return backupClient, nil
		},
		confirm:      func(io.Reader) bool { return true },
		promptWriter: &prompt,
	}

	require.NoError(t, runner.run(context.Background(), updateTrustPolicyFlags{clusterID: "hcp", replacementMC: "replacement"}))
	assert.Contains(t, prompt.String(), "Source management cluster: source-name (source-id)")
	assert.Contains(t, prompt.String(), "Replacement management cluster: replacement-name (replacement-id)")
}

func TestTrustPolicyUpdateRunnerRejectsReplacementBackupAccountMismatch(t *testing.T) {
	t.Parallel()

	runner := &trustPolicyUpdateRunner{
		resolver: &staticClusterResolver{clusterInfo: ClusterInfo{HCPClusterID: "hcp-id", HCPClusterRegion: "us-east-1", MgmtClusterID: "source-id", MgmtClusterName: "source-name"}},
		getCluster: func(identifier string) (*cmv1.Cluster, error) {
			if identifier != "replacement" {
				return nil, fmt.Errorf("unexpected identifier %s", identifier)
			}
			return newTestCluster(t, "replacement-id", "replacement-name"), nil
		},
		getBackupConfig: func(_ context.Context, _ string, managementCluster string) (*cmv1.AWSBackupConfig, error) {
			switch managementCluster {
			case "source-name":
				return backupConfig(t, managementCluster, "source-bucket", "123456789012"), nil
			case "replacement-name":
				return backupConfig(t, managementCluster, "replacement-bucket", "210987654321"), nil
			default:
				return nil, fmt.Errorf("unexpected management cluster %s", managementCluster)
			}
		},
	}

	err := runner.run(context.Background(), updateTrustPolicyFlags{clusterID: "hcp", replacementMC: "replacement"})
	assert.ErrorContains(t, err, "uses backup account 210987654321, expected 123456789012")
}

func TestTrustPolicyUpdateRunnerResolvesSourceIdentifier(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	baseClient := awsmock.NewMockClient(ctrl)
	backupClient := awsmock.NewMockClient(ctrl)
	document := `{"Statement":[{"Action":"sts:AssumeRoleWithWebIdentity","Principal":{"Federated":"arn:aws:iam::1:oidc-provider/issuer/source-id"},"Condition":{"StringEquals":{"issuer/source-id:sub":"subject"}}}]}`
	backupClient.EXPECT().GetRole(gomock.Cond(func(input *iam.GetRoleInput) bool {
		return awssdk.ToString(input.RoleName) == "rosa-hcp-bkp-canonical-source-hcp-id"
	})).Return(&iam.GetRoleOutput{Role: &iamtypes.Role{AssumeRolePolicyDocument: awssdk.String(document)}}, nil)
	backupClient.EXPECT().UpdateAssumeRolePolicy(gomock.Any()).Return(&iam.UpdateAssumeRolePolicyOutput{}, nil)

	runner := &trustPolicyUpdateRunner{
		resolver: &staticClusterResolver{clusterInfo: ClusterInfo{HCPClusterID: "hcp-id", HCPClusterRegion: "us-east-1", MgmtClusterID: "current-id", MgmtClusterName: "current-name"}},
		getCluster: func(identifier string) (*cmv1.Cluster, error) {
			switch identifier {
			case "source-external":
				return newTestCluster(t, "source-id", "canonical-source"), nil
			case "replacement-external":
				return newTestCluster(t, "replacement-id", "canonical-replacement"), nil
			default:
				return nil, fmt.Errorf("unexpected identifier %s", identifier)
			}
		},
		getBackupConfig: func(_ context.Context, _ string, managementCluster string) (*cmv1.AWSBackupConfig, error) {
			return backupConfig(t, managementCluster, managementCluster+"-bucket", "123456789012"), nil
		},
		newAWSClient:        func(string, string, string) (awsprovider.Client, error) { return baseClient, nil },
		generateSessionName: func(awsprovider.Client) (string, error) { return "session", nil },
		getAWSPartition:     func(awsprovider.Client) (string, error) { return "aws", nil },
		generateCredentials: func(awsprovider.Client, string, string, string) (*ststypes.Credentials, error) {
			return &ststypes.Credentials{AccessKeyId: awssdk.String("key"), SecretAccessKey: awssdk.String("secret"), SessionToken: awssdk.String("token")}, nil
		},
		newAWSClientInput: func(*awsprovider.ClientInput) (awsprovider.Client, error) { return backupClient, nil },
		confirm:           func(io.Reader) bool { return true },
		in:                strings.NewReader("yes\n"),
		promptWriter:      io.Discard,
	}

	require.NoError(t, runner.run(context.Background(), updateTrustPolicyFlags{clusterID: "hcp", sourceMC: "source-external", replacementMC: "replacement-external"}))
}

func newTestCluster(t *testing.T, id, name string) *cmv1.Cluster {
	t.Helper()
	cluster, err := cmv1.NewCluster().ID(id).Name(name).Build()
	require.NoError(t, err)
	return cluster
}

func TestUpdateTrustPolicyFlags(t *testing.T) {
	t.Parallel()
	cmd := newCmdUpdateTrustPolicy()
	assert.NotNil(t, cmd.Flags().Lookup("cluster-id"))
	assert.NotNil(t, cmd.Flags().Lookup("source-mc"))
	assert.NotNil(t, cmd.Flags().Lookup("replacement-mc"))
	assert.NotNil(t, cmd.Flags().Lookup("profile"))
}

func TestConfirmTrustPolicyUpdate(t *testing.T) {
	t.Parallel()
	assert.True(t, confirmTrustPolicyUpdate(strings.NewReader("yes\n")))
	assert.False(t, confirmTrustPolicyUpdate(strings.NewReader("no\n")))
}
