package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	ocmsdk "github.com/openshift-online/ocm-sdk-go"
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

func TestOCMClientConfig(t *testing.T) {
	var config OCMClientConfig
	config.Options()
	config.Defaults()

	assert.NotNil(t, config.Logger)
}

func TestManagementClusterAssignment(t *testing.T) {
	assignment := "source-mc"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"eyJhbGciOiJub25lIn0.eyJleHAiOjQxMDAwMDAwMDB9.signature","expires_in":3600}`)) // notsecret
		case "/api/clusters_mgmt/v1/clusters/hcp-id/hypershift":
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(`{"management_cluster":"` + assignment + `"}`))
			case http.MethodPatch:
				body, err := io.ReadAll(r.Body)
				if !assert.NoError(t, err) {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				assert.JSONEq(t, `{"management_cluster":"replacement-mc"}`, string(body))
				assignment = "replacement-mc"
				_, _ = w.Write([]byte(`{"management_cluster":"replacement-mc"}`))
			default:
				t.Errorf("unexpected request method %s", r.Method)
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			t.Errorf("unexpected request path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	conn, err := ocmsdk.NewConnectionBuilder().
		URL(server.URL).
		TokenURL(server.URL+"/token").
		Insecure(true).
		Client("client", "secret").
		Build()
	require.NoError(t, err)
	defer conn.Close()

	client := NewOCMClient(conn)
	current, err := client.ManagementClusterAssignment(context.Background(), "hcp-id")
	require.NoError(t, err)
	assert.Equal(t, "source-mc", current)

	require.NoError(t, client.UpdateManagementClusterAssignment(context.Background(), "hcp-id", "replacement-mc"))
	current, err = client.ManagementClusterAssignment(context.Background(), "hcp-id")
	require.NoError(t, err)
	assert.Equal(t, "replacement-mc", current)
}
