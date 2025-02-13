package mustgather

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"

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
	err := os.MkdirAll(dir, 0755)
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
