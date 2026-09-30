package backup

import (
	"context"
	"fmt"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	awsmock "github.com/openshift/osdctl/pkg/provider/aws/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func backupConfig(t *testing.T, managementCluster, bucket, accountID string) *cmv1.AWSBackupConfig {
	t.Helper()
	config, err := cmv1.NewAWSBackupConfig().
		ManagementCluster(managementCluster).
		S3Bucket(bucket).
		AccountId(accountID).
		Build()
	require.NoError(t, err)
	return config
}

func TestSelectBackupConfig(t *testing.T) {
	t.Parallel()

	matching := backupConfig(t, "hs-mc-a", "backup-bucket", "123456789012")
	other := backupConfig(t, "hs-mc-b", "other-bucket", "210987654321")
	incomplete := backupConfig(t, "hs-mc-a", "", "")

	tests := []struct {
		name    string
		configs []*cmv1.AWSBackupConfig
		wantErr string
	}{
		{name: "matching config", configs: []*cmv1.AWSBackupConfig{other, matching}},
		{name: "no matching config", configs: []*cmv1.AWSBackupConfig{other}, wantErr: "no backup configuration"},
		{name: "duplicate matching config", configs: []*cmv1.AWSBackupConfig{matching, matching}, wantErr: "multiple backup configurations"},
		{name: "incomplete matching config", configs: []*cmv1.AWSBackupConfig{incomplete}, wantErr: "is incomplete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectBackupConfig(tt.configs, "hs-mc-a")
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Same(t, matching, got)
		})
	}
}

func TestParseBackupPrefix(t *testing.T) {
	t.Parallel()

	const parent = "cluster/backups/"
	tests := []struct {
		name   string
		prefix string
		wantID string
	}{
		{name: "hourly backup", prefix: parent + "cluster-1-hourly-20260930190050/", wantID: "cluster-1-hourly-20260930190050"},
		{name: "daily backup", prefix: parent + "cluster-1-daily-20260930050050/", wantID: "cluster-1-daily-20260930050050"},
		{name: "wrong parent", prefix: "other/backups/cluster-1-daily-20260930050050/"},
		{name: "nested prefix", prefix: parent + "cluster-1-daily-20260930050050/metadata/"},
		{name: "malformed timestamp", prefix: parent + "cluster-1-daily-not-a-time/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseBackupPrefix(parent, tt.prefix)
			assert.Equal(t, tt.wantID != "", ok)
			assert.Equal(t, tt.wantID, got.id)
		})
	}
}

func TestParseBackupKey(t *testing.T) {
	t.Parallel()

	const parent = "2srl9k3p09385avt1gk1o7vup4iuc9qs/backups/"
	backup, ok := parseBackupKey(parent, parent+"2srl9k3p09385avt1gk1o7vup4iuc9qs-1-hourly-20260930190050/metadata.json")
	require.True(t, ok)
	assert.Equal(t, "2srl9k3p09385avt1gk1o7vup4iuc9qs-1-hourly-20260930190050", backup.id)
}

func TestListBackupsPaginatesAndSorts(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := awsmock.NewMockClient(ctrl)
	firstToken := "next"
	client.EXPECT().ListObjectsV2(gomock.Cond(func(input *s3.ListObjectsV2Input) bool {
		return awssdk.ToString(input.Bucket) == "backup-bucket" &&
			awssdk.ToString(input.Prefix) == "cluster/backups/" &&
			awssdk.ToString(input.Delimiter) == "/" &&
			input.ContinuationToken == nil
	})).Return(&s3.ListObjectsV2Output{
		IsTruncated:           awssdk.Bool(true),
		NextContinuationToken: &firstToken,
		CommonPrefixes:        []types.CommonPrefix{{Prefix: awssdk.String("cluster/backups/cluster-daily-20260930050050/")}},
	}, nil)
	client.EXPECT().ListObjectsV2(gomock.Cond(func(input *s3.ListObjectsV2Input) bool {
		return awssdk.ToString(input.ContinuationToken) == firstToken
	})).Return(&s3.ListObjectsV2Output{CommonPrefixes: []types.CommonPrefix{
		{Prefix: awssdk.String("cluster/backups/cluster-hourly-20260930190050/")},
		{Prefix: awssdk.String("cluster/backups/not-a-backup/")},
	}, Contents: []types.Object{
		{Key: awssdk.String("cluster/backups/cluster-hourly-20260930190050/metadata.json")},
		{Key: awssdk.String("cluster/backups/cluster-hourly-20260928190050/metadata.json")},
	}}, nil)

	backups, err := listBackups(client, "backup-bucket", "cluster")
	require.NoError(t, err)
	assert.Equal(t, []string{"cluster-hourly-20260930190050", "cluster-daily-20260930050050", "cluster-hourly-20260928190050"}, []string{backups[0].id, backups[1].id, backups[2].id})
}

func TestBackupDiscoveryRunnerPrintsRequestedBackups(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		limit int
		want  string
	}{
		{name: "newest", limit: 1, want: "cluster-hourly-20260930190050\n"},
		{name: "two newest", limit: 2, want: "cluster-hourly-20260930190050\ncluster-hourly-20260929190050\n"},
		{name: "all", limit: -1, want: "cluster-hourly-20260930190050\ncluster-hourly-20260929190050\n"},
		{name: "more than available", limit: 3, want: "cluster-hourly-20260930190050\ncluster-hourly-20260929190050\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			baseClient := awsmock.NewMockClient(ctrl)
			s3Client := awsmock.NewMockClient(ctrl)
			s3Client.EXPECT().ListObjectsV2(gomock.Any()).Return(&s3.ListObjectsV2Output{CommonPrefixes: []types.CommonPrefix{
				{Prefix: awssdk.String("cluster/backups/cluster-hourly-20260929190050/")},
				{Prefix: awssdk.String("cluster/backups/cluster-hourly-20260930190050/")},
			}}, nil)

			var output strings.Builder
			runner := &backupDiscoveryRunner{
				resolver: &staticClusterResolver{clusterInfo: ClusterInfo{
					HCPClusterID:     "cluster",
					HCPClusterRegion: "us-east-1",
					MgmtClusterName:  "hs-mc-a",
				}},
				getBackupConfig: func(_ context.Context, clusterID, managementCluster string) (*cmv1.AWSBackupConfig, error) {
					assert.Equal(t, "cluster", clusterID)
					assert.Equal(t, "hs-mc-a", managementCluster)
					return backupConfig(t, managementCluster, "backup-bucket", "123456789012"), nil
				},
				newAWSClient: func(profile, region, configFile string) (awsprovider.Client, error) {
					assert.Empty(t, profile)
					assert.Equal(t, "us-east-1", region)
					assert.Empty(t, configFile)
					return baseClient, nil
				},
				generateSessionName: func(client awsprovider.Client) (string, error) {
					assert.Same(t, baseClient, client)
					return "session", nil
				},
				getAWSPartition: func(client awsprovider.Client) (string, error) {
					assert.Same(t, baseClient, client)
					return "aws", nil
				},
				generateCredentials: func(client awsprovider.Client, accountID, sessionName, partition string) (*ststypes.Credentials, error) {
					assert.Same(t, baseClient, client)
					assert.Equal(t, "123456789012", accountID)
					assert.Equal(t, "session", sessionName)
					assert.Equal(t, "aws", partition)
					return &ststypes.Credentials{AccessKeyId: awssdk.String("key"), SecretAccessKey: awssdk.String("secret"), SessionToken: awssdk.String("token")}, nil
				},
				newAWSClientInput: func(input *awsprovider.ClientInput) (awsprovider.Client, error) {
					assert.Equal(t, &awsprovider.ClientInput{AccessKeyID: "key", SecretAccessKey: "secret", SessionToken: "token", Region: "us-east-1"}, input)
					return s3Client, nil
				},
				printer: &defaultPrinter{w: &output},
			}

			require.NoError(t, runner.run(context.Background(), discoveryFlags{clusterID: "cluster", limit: tt.limit}))
			assert.Equal(t, tt.want, output.String())
		})
	}
}

func TestBackupDiscoveryRunnerRejectsInvalidLimit(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{0, -2} {
		t.Run(fmt.Sprintf("limit %d", limit), func(t *testing.T) {
			runner := &backupDiscoveryRunner{}
			err := runner.run(context.Background(), discoveryFlags{limit: limit})
			assert.ErrorContains(t, err, "limit must be a positive number or -1")
		})
	}
}

func TestDiscoveryFlags(t *testing.T) {
	t.Parallel()

	cmd := newCmdDiscover()
	assert.NotNil(t, cmd.Flags().Lookup("cluster-id"))
	assert.NotNil(t, cmd.Flags().Lookup("limit"))
	assert.NotNil(t, cmd.Flags().Lookup("profile"))
}
