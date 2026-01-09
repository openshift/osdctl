package k8s

import (
	"context"
	"fmt"
	"io"

	sdk "github.com/openshift-online/ocm-sdk-go"
	bplogin "github.com/openshift/backplane-cli/cmd/ocm-backplane/login"
	bpconfig "github.com/openshift/backplane-cli/pkg/cli/config"
	bputils "github.com/openshift/backplane-cli/pkg/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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
	setRuntimeLoggerDiscard()
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
	cfg, err := NewRestConfig(clusterID)
	if err != nil {
		return nil, err
	}
	setRuntimeLoggerDiscard()
	return client.New(cfg, options)
}

// NewRestConfig returns a *rest.Config for the given cluster ID using backplane configuration
func NewRestConfig(clusterID string) (*rest.Config, error) {
	bp, err := bpconfig.GetBackplaneConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to load backplane-cli config: %v", err)
	}

	cfg, err := bplogin.GetRestConfig(bp, clusterID)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// Create Backplane connection to a provided cluster, using a provided ocm sdk connection
// This is intended to allow backplane connections to multiple clusters which exist in different
// ocm environments by allowing the caller to provide an ocm connection to the function.
func NewWithConn(clusterID string, options client.Options, ocmConn *sdk.Connection) (client.Client, error) {
	if ocmConn == nil {
		return nil, fmt.Errorf("nil OCM sdk connection provided to NewWithConn()")
	}
	bp, err := bpconfig.GetBackplaneConfigurationWithConn(ocmConn)
	if err != nil {
		return nil, fmt.Errorf("failed to load backplane-cli config: %v", err)
	}

	cfg, err := bplogin.GetRestConfigWithConn(bp, ocmConn, clusterID)
	if err != nil {
		return nil, err
	}
	setRuntimeLoggerDiscard()
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
	setRuntimeLoggerDiscard()
	return client.New(cfg, options)
}

// Create Backplane connection as cluster admin to a provided cluster, using a provided ocm sdk connection
// This is intended to allow backplane connections to multiple clusters which exist in different
// ocm environments by allowing the caller to provide an ocm connection to the function.
func NewAsBackplaneClusterAdminWithConn(clusterID string, options client.Options, ocmConn *sdk.Connection, elevationReasons ...string) (client.Client, error) {
	if ocmConn == nil {
		return nil, fmt.Errorf("nil OCM sdk connection provided to NewAsBackplaneClusterAdminWithConn()")
	}
	bp, err := bpconfig.GetBackplaneConfigurationWithConn(ocmConn)
	if err != nil {
		return nil, fmt.Errorf("failed to load backplane-cli config: %v", err)
	}

	cfg, err := bplogin.GetRestConfigAsUserWithConn(bp, ocmConn, clusterID, "backplane-cluster-admin", elevationReasons...)
	if err != nil {
		return nil, err
	}
	setRuntimeLoggerDiscard()
	return client.New(cfg, options)
}

func setRuntimeLoggerDiscard() {
	// To avoid warnings/backtrace, if k8s controller-runtime logger has not already been set, do it now...
	if !log.Log.Enabled() {
		log.SetLogger(zap.New(zap.WriteTo(io.Discard)))
	}
}

func GetCurrentCluster() (string, error) {
	cluster, err := bputils.DefaultClusterUtils.GetBackplaneClusterFromConfig()
	if err != nil {
		return "", fmt.Errorf("failed to retrieve backplane status: %v", err)
	}
	return cluster.ClusterID, nil
}

func LazyClientInit(fc client.WithWatch) *LazyClient {
	return &LazyClient{
		client: fc,
	}
}

func LazyClientMock(c client.Client) *LazyClient {
	return &LazyClient{
		client: c,
	}
}
