package cmd

import (
	"context"
	"fmt"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
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
		want  []string
	}{
		{name: "newest", limit: 1, want: []string{"cluster-hourly-20260930190050"}},
		{name: "two newest", limit: 2, want: []string{"cluster-hourly-20260930190050", "cluster-hourly-20260929190050"}},
		{name: "all", limit: -1, want: []string{"cluster-hourly-20260930190050", "cluster-hourly-20260929190050"}},
		{name: "more than available", limit: 3, want: []string{"cluster-hourly-20260930190050", "cluster-hourly-20260929190050"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s3Client := awsmock.NewMockClient(ctrl)
			s3Client.EXPECT().ListObjectsV2(gomock.Any()).Return(&s3.ListObjectsV2Output{CommonPrefixes: []types.CommonPrefix{
				{Prefix: awssdk.String("cluster/backups/cluster-hourly-20260929190050/")},
				{Prefix: awssdk.String("cluster/backups/cluster-hourly-20260930190050/")},
			}}, nil)

			awsClient := &testAWSAccountClientProvider{client: s3Client}
			discoverer := &DefaultBackupDiscoverer{
				ocmClient: &testDiscoveryOCMClient{cluster: ClusterInfo{
					HCPClusterID:     "cluster",
					HCPClusterRegion: "us-east-1",
					MgmtClusterName:  "hs-mc-a",
				}, config: backupConfig(t, "hs-mc-a", "backup-bucket", "123456789012")},
				awsClient: awsClient,
			}

			backups, err := discoverer.DiscoverBackups(context.Background(), DiscoveryRequest{ClusterID: "cluster", Limit: tt.limit})
			require.NoError(t, err)
			assert.Equal(t, []awsAccountClientCall{{region: "us-east-1", accountID: "123456789012"}}, awsClient.calls)
			ids := make([]string, 0, len(backups))
			for _, backup := range backups {
				ids = append(ids, backup.ID)
			}
			assert.Equal(t, tt.want, ids)
		})
	}
}

func TestBackupDiscoveryRunnerRejectsInvalidLimit(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{0, -2} {
		t.Run(fmt.Sprintf("limit %d", limit), func(t *testing.T) {
			discoverer := &DefaultBackupDiscoverer{}
			_, err := discoverer.DiscoverBackups(context.Background(), DiscoveryRequest{Limit: limit})
			assert.ErrorContains(t, err, "limit must be a positive number or -1")
		})
	}
}
