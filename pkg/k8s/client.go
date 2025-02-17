package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	bplogin "github.com/openshift/backplane-cli/cmd/ocm-backplane/login"
	bpapi "github.com/openshift/backplane-cli/pkg/backplaneapi"
	bpconfig "github.com/openshift/backplane-cli/pkg/cli/config"
	bputils "github.com/openshift/backplane-cli/pkg/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type lazyClientInitializerInterface interface {
	initialize(s *LazyClient)
}

type lazyClientInitializer struct {
	lazyClientInitializerInterface
}

func (b *lazyClientInitializer) initialize(s *LazyClient) {
	configLoader := s.flags.ToRawKubeConfigLoader()
	cfg, err := configLoader.ClientConfig()
	if err != nil {
		//The stub is to allow commands that don't need a connection to a Kubernetes cluster.
		//We'll produce a warning and the stub itself will error when a command is trying to use it.
		panic(s.err())
	}
	if len(s.userName) > 0 || len(s.elevationReasons) > 0 {
		if len(s.userName) == 0 {
			s.userName = "backplane-cluster-admin"
		}
		impersonationConfig := rest.ImpersonationConfig{
			UserName: s.userName,
		}
		if len(s.elevationReasons) > 0 {
			impersonationConfig.Extra = map[string][]string{"reason": s.elevationReasons}
		}
		cfg.Impersonate = impersonationConfig
	}

	s.client, err = client.New(cfg, client.Options{})
	if err != nil {
		panic(s.err())
	}
}

type LazyClient struct {
	lazyClientInitializer lazyClientInitializerInterface
	client                client.Client
	flags                 *genericclioptions.ConfigFlags
	userName              string
	elevationReasons      []string
}

// GroupVersionKindFor implements client.Client.
func (*LazyClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	panic("unimplemented")
}

// IsObjectNamespaced implements client.Client.
func (*LazyClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	panic("unimplemented")
}

func (s *LazyClient) Scheme() *runtime.Scheme {
	return s.client.Scheme()
}

func (s *LazyClient) RESTMapper() meta.RESTMapper {
	return s.client.RESTMapper()
}

func (s *LazyClient) err() error {
	return fmt.Errorf("not connected to real cluster, please verify KUBECONFIG is correct")
}

func (s *LazyClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return s.getClient().Get(ctx, key, obj)
}

func (s *LazyClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return s.getClient().List(ctx, list, opts...)
}

func (s *LazyClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return s.getClient().Create(ctx, obj, opts...)
}

func (s *LazyClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return s.getClient().Delete(ctx, obj, opts...)
}

func (s *LazyClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return s.getClient().Update(ctx, obj, opts...)
}

func (s *LazyClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return s.getClient().Patch(ctx, obj, patch, opts...)
}

func (s *LazyClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return s.getClient().DeleteAllOf(ctx, obj, opts...)
}

func (s *LazyClient) Status() client.StatusWriter {
	return s.getClient().Status()
}

func (s *LazyClient) SubResource(subResource string) client.SubResourceClient {
	return s.getClient().SubResource(subResource)
}

func (s *LazyClient) Impersonate(userName string, elevationReasons ...string) {
	if s.client != nil {
		panic("cannot impersonate a client which has been already initialized")
	}
	s.userName = userName
	s.elevationReasons = elevationReasons
}

func NewClient(flags *genericclioptions.ConfigFlags) *LazyClient {
	return &LazyClient{&lazyClientInitializer{}, nil, flags, "", nil}
}

func (s *LazyClient) getClient() client.Client {
	if s.client == nil {
		s.lazyClientInitializer.initialize(s)
	}
	return s.client
}

func New(clusterID string, options client.Options) (client.Client, error) {
	bp, err := bpconfig.GetBackplaneConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to load backplane-cli config: %v", err)
	}

	cfg, err := bplogin.GetRestConfig(bp, clusterID)
	if err != nil {
		return nil, err
	}

	return client.New(cfg, options)
}

func NewAsBackplaneClusterAdmin(clusterID string, options client.Options, elevationReasons ...string) (client.Client, error) {
	bp, err := bpconfig.GetBackplaneConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to load backplane-cli config: %v", err)
	}

	cfg, err := bplogin.GetRestConfigAsUser(bp, clusterID, "backplane-cluster-admin", elevationReasons...)
	if err != nil {
		return nil, err
	}

	return client.New(cfg, options)
}

func GetCurrentCluster() (string, error) {
	cluster, err := bputils.DefaultClusterUtils.GetBackplaneClusterFromConfig()
	if err != nil {
		return "", fmt.Errorf("failed to retrieve backplane status: %v", err)
	}
	return cluster.ClusterID, nil
}

/* Following is used to support multiple ocm envs in a single tool by allowing the OCM config to be passed as a file and then as an object.
 * The most common case for this is when a tool needs to access a cluster outside of 'prod' such as 'staging' or 'integration',
 * but the hive shard exists in 'prod'.
 */

// Borrowed from backplane-cli/config, repurposed here to allow the backplane url to be provided as an arg instead of
// discovering the url from the OCM environment vars at the time executed.
// Test proxy urls, return first proxy-url that results in a successful request to the <backplaneBaseUrl>/healthz endpoint
func GetFirstWorkingProxyURL(bpBaseURL string, proxyURLs []string, debug bool) string {
	bpHealthzURL := bpBaseURL + "/healthz"

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	for _, testProxy := range proxyURLs {
		proxyURL, err := url.ParseRequestURI(testProxy)
		if err != nil {
			// Warn user against proxy aliases such as 'prod', 'stage', etc. in config
			// so they can resolve to proper URLs (ie https://...openshift.com)
			if debug {
				fmt.Fprintf(os.Stderr, "proxy-url: '%v' could not be parsed as URI. Proxy Aliases not yet supported", testProxy)
			}
			continue
		}

		client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		req, _ := http.NewRequest("GET", bpHealthzURL, nil)
		resp, err := client.Do(req)
		if err != nil {
			if debug {
				fmt.Fprintf(os.Stderr, "Proxy: %s returned an error: %s", proxyURL, err)
			}
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return testProxy
		}
		if debug {
			fmt.Fprintf(os.Stderr, "proxy: %s did not pass healthcheck, expected response code 200, got %d, discarding", testProxy, resp.StatusCode)
		}
	}
	fmt.Fprintf(os.Stderr, "Failed to find a working proxy-url for backplane request path:'%s'", bpHealthzURL)
	if len(proxyURLs) > 0 {
		fmt.Fprintf(os.Stderr, "falling back to first proxy-url after all proxies failed health checks: %s", proxyURLs[0])
		return proxyURLs[0]
	}
	return ""
}

// BackplaneLogin returns the proxy url for the target cluster.
// borrowed from backplane-cli/ocm-backplane/login and repurposed here to
// support providing token and proxy instead of pulling these from the env vars in the chain of vendored functions.
func GetBPAPIClusterLoginProxyUri(BPAPIUrl string, clusterID string, accessToken string, proxyURL string) (string, error) {
	// This ends up using 'ocm.DefaultOCMInterface.GetOCMEnvironment()' to get the backplane url from OCM
	//client, err := backplaneapi.DefaultClientUtils.MakeRawBackplaneAPIClientWithAccessToken(api, accessToken)
	var proxyArg *string
	if len(proxyURL) > 0 {
		proxyArg = &proxyURL
	}
	client, err := bpapi.DefaultClientUtils.GetBackplaneClient(BPAPIUrl, accessToken, proxyArg)
	if err != nil {
		return "", fmt.Errorf("unable to create backplane api client")
	}

	resp, err := client.LoginCluster(context.TODO(), clusterID)
	// Print the whole response if we can't parse it. Eg. 5xx error from http server.
	if err != nil {
		// trying to determine the error
		errBody := err.Error()
		if strings.Contains(errBody, "dial tcp") && strings.Contains(errBody, "i/o timeout") {
			return "", fmt.Errorf("unable to connect to backplane api")
		}
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		strErr, err := tryParseBackplaneAPIErrorMsg(resp)
		if err != nil {
			return "", fmt.Errorf("failed to parse response error. Parse err:'%v'", err)
		}
		return "", fmt.Errorf("loginCluster() response error:'%s'", strErr)
	}
	//respProxyUri, err := bpclient.ParseLoginClusterResponse(resp)
	respProxyUri, err := getClusterResponseProxyUri(resp)
	if err != nil {
		return "", fmt.Errorf("unable to parse response body from backplane: \n Status Code: %d", resp.StatusCode)
	}
	// return api + *loginResp.JSON200.ProxyUri, nil
	return BPAPIUrl + respProxyUri, nil
}

// Borrowed from backplaneApi LoginResponse
// LoginResponse Login status response
type loginResponse struct {
	// Message message
	Message *string `json:"message,omitempty"`

	// ProxyUri KubeAPI proxy URI
	ProxyUri *string `json:"proxy_uri,omitempty"`

	// StatusCode status code
	StatusCode *int `json:"statusCode,omitempty"`
}

// Intended to parse ProxyUri from login response avoiding
// avoids vendoring 'github.com/openshift/backplane-api/pkg/client'
func getClusterResponseProxyUri(rsp *http.Response) (string, error) {
	bodyBytes, err := io.ReadAll(rsp.Body)
	defer func() { _ = rsp.Body.Close() }()
	if err != nil {
		return "", err
	}
	switch {
	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
		var dest loginResponse
		if err := json.Unmarshal(bodyBytes, &dest); err != nil {
			return "", err
		}
		return *dest.ProxyUri, nil

	}
	// Calling function should check status code , but log here just in case.
	if rsp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Did not parse ProxyUri from cluster login response. resp code:'%d'", rsp.StatusCode)
	}
	return "", nil
}

// Borrowed from backplaneApi Error
// Error defines model for Error.
type bpError struct {
	// Message Error Message
	Message *string `json:"message,omitempty"`

	// StatusCode HTTP status code
	StatusCode *int `json:"statusCode,omitempty"`
}

func tryParseBackplaneAPIErrorMsg(rsp *http.Response) (string, error) {
	bodyBytes, err := io.ReadAll(rsp.Body)
	defer func() { _ = rsp.Body.Close() }()
	if err != nil {
		return "", err
	}
	var dest bpError
	if err := json.Unmarshal(bodyBytes, &dest); err != nil {
		return "", err
	}
	if dest.Message != nil && dest.StatusCode != nil {
		return fmt.Sprintf("error from backplane: \n Status Code: %d\n Message: %s", *dest.StatusCode, *dest.Message), nil
	} else {
		return fmt.Sprintf("error from backplane: \n Status Code: %d\n Message: %s", rsp.StatusCode, rsp.Status), nil
	}
}
