package k8s

import (
	"context"
	"fmt"

	bplogin "github.com/openshift/backplane-cli/cmd/ocm-backplane/login"
	bpconfig "github.com/openshift/backplane-cli/pkg/cli/config"
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
