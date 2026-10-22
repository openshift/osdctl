package client

import (
	"testing"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectBackupConfig(t *testing.T) {
	config := func(t *testing.T, managementCluster, bucket, accountID string) *cmv1.AWSBackupConfig {
		t.Helper()
		result, err := cmv1.NewAWSBackupConfig().ManagementCluster(managementCluster).S3Bucket(bucket).AccountId(accountID).Build()
		require.NoError(t, err)
		return result
	}
	matching := config(t, "hs-mc-a", "backup-bucket", "123456789012")
	other := config(t, "hs-mc-b", "other-bucket", "210987654321")
	incomplete := config(t, "hs-mc-a", "", "")
	for _, tt := range []struct {
		name, wantErr string
		configs       []*cmv1.AWSBackupConfig
	}{
		{name: "matching", configs: []*cmv1.AWSBackupConfig{other, matching}},
		{name: "missing", configs: []*cmv1.AWSBackupConfig{other}, wantErr: "no backup configuration"},
		{name: "duplicate", configs: []*cmv1.AWSBackupConfig{matching, matching}, wantErr: "multiple backup configurations"},
		{name: "incomplete", configs: []*cmv1.AWSBackupConfig{incomplete}, wantErr: "is incomplete"},
	} {
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
