package mustgather

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func getMockRestConfig() *rest.Config {
	return &rest.Config{
		Host: "https://example.com",
		TLSClientConfig: rest.TLSClientConfig{
			CAFile:   "/path/to/ca.crt",
			CertFile: "/path/to/cert.crt",
			KeyFile:  "/path/to/key.key",
		},
		BearerToken: "some-token",
		Impersonate: rest.ImpersonationConfig{
			UserName: "testuser",
			Extra:    map[string][]string{"reason": {"test"}},
		},
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse("http://proxy.example.com")
		},
	}
}

func TestCreateKubeconfigFileForRestConfig_Success(t *testing.T) {
	mockConfig := getMockRestConfig()

	// Create a tmp kubeconfig & delete after we're done
	kubeConfigFile := createKubeconfigFileForRestConfig(mockConfig)
	defer os.Remove(kubeConfigFile)

	_, err := os.Stat(kubeConfigFile)
	assert.NoError(t, err)

	config, err := clientcmd.LoadFromFile(kubeConfigFile)
	assert.NoError(t, err)

	cluster, ok := config.Clusters["default-cluster"]
	assert.True(t, ok)
	assert.Equal(t, mockConfig.Host, cluster.Server)

	assert.Equal(t, "http://proxy.example.com", cluster.ProxyURL)

	context, ok := config.Contexts["default-context"]
	assert.True(t, ok)
	assert.Equal(t, "default-cluster", context.Cluster)
	assert.Equal(t, "default-user", context.AuthInfo)

	authInfo, ok := config.AuthInfos["default-user"]
	assert.True(t, ok)
	assert.Equal(t, mockConfig.BearerToken, authInfo.Token)
	assert.Equal(t, "testuser", authInfo.Impersonate)
	assert.Equal(t, map[string][]string{"reason": {"test"}}, authInfo.ImpersonateUserExtra)
}

// Test creating a tarball with actual files in a directory
func TestCreateTarball_Success(t *testing.T) {
	dir := "/tmp/osdctl-targz-test"
	err := os.MkdirAll(dir, 0750)
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// Create a test file
	fileName := fmt.Sprintf("%s/testfile.txt", dir)
	file, err := os.Create(fileName)
	require.NoError(t, err)
	defer file.Close()
	_, err = file.WriteString("Hello, world!")
	require.NoError(t, err)

	// Create the tarball
	tarballName := fmt.Sprintf("%s/osdctl-targz-test-output-testdata.tar.gz", "/tmp")
	err = createTarball(dir, tarballName)
	require.NoError(t, err)

	// Check tarball creation
	_, err = os.Stat(tarballName)
	assert.NoError(t, err)

}

// Test tarball creation failure when directory doesn't exist
func TestCreateTarball_FileError(t *testing.T) {
	err := createTarball("/nonexistent/path", "/tmp/testdata.tar.gz")
	assert.Error(t, err)
}

func mustCluster(t *testing.T, id string, hypershift bool) *cmv1.Cluster {
	t.Helper()
	cluster, err := cmv1.NewCluster().
		ID(id).
		Hypershift(cmv1.NewHypershift().Enabled(hypershift)).
		Build()
	require.NoError(t, err)
	return cluster
}

func TestParseAndValidateTargets(t *testing.T) {
	hcp := mustCluster(t, "hcp-id", true)
	nonHcp := mustCluster(t, "mc-id", false)

	tests := []struct {
		name     string
		raw      string
		cluster  *cmv1.Cluster
		want     []string
		errMatch string
	}{
		{name: "default hcp on HCP cluster", raw: "hcp", cluster: hcp, want: []string{"hcp"}},
		{name: "all targets on HCP cluster", raw: "hcp,mc,sc,sc_acm", cluster: hcp, want: []string{"hcp", "mc", "sc", "sc_acm"}},
		{name: "trims whitespace", raw: " mc , hcp ", cluster: hcp, want: []string{"mc", "hcp"}},
		{name: "de-duplicates repeats", raw: "hcp,hcp,mc", cluster: hcp, want: []string{"hcp", "mc"}},
		{name: "mc directly against non-HCP cluster", raw: "mc", cluster: nonHcp, want: []string{"mc"}},
		{name: "sc_acm against non-HCP cluster", raw: "sc,sc_acm", cluster: nonHcp, want: []string{"sc", "sc_acm"}},
		{name: "unknown target", raw: "mc,foo", cluster: hcp, errMatch: "unknown gather target"},
		{name: "empty", raw: " , ", cluster: hcp, errMatch: "no gather targets specified"},
		{name: "hcp against non-HCP cluster", raw: "hcp", cluster: nonHcp, errMatch: "requires an HCP cluster ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAndValidateTargets(tt.raw, tt.cluster)
			if tt.errMatch != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMatch)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSummarizeResults(t *testing.T) {
	res := func(s gatherStatus) targetResult { return targetResult{status: s} }

	tests := []struct {
		name         string
		results      []targetResult
		wantProduced bool
		wantAllOK    bool
	}{
		{name: "all succeeded", results: []targetResult{res(statusSucceeded), res(statusSucceeded)}, wantProduced: true, wantAllOK: true},
		{name: "succeeded and failed", results: []targetResult{res(statusSucceeded), res(statusFailed)}, wantProduced: true, wantAllOK: false},
		{name: "single partial", results: []targetResult{res(statusPartial)}, wantProduced: true, wantAllOK: false},
		{name: "all failed", results: []targetResult{res(statusFailed), res(statusFailed)}, wantProduced: false, wantAllOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			produced, allOK := summarizeResults(tt.results)
			assert.Equal(t, tt.wantProduced, produced)
			assert.Equal(t, tt.wantAllOK, allOK)
		})
	}
}
