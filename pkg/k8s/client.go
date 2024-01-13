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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LazyClient struct {
	client client.Client
	flags  *genericclioptions.ConfigFlags
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

func NewClient(flags *genericclioptions.ConfigFlags) client.Client {
	return &LazyClient{nil, flags}
}

func (s *LazyClient) getClient() client.Client {
	if s.client == nil {
		s.initialize()
	}
	return s.client
}

func (s *LazyClient) initialize() {
	configLoader := s.flags.ToRawKubeConfigLoader()
	cfg, err := configLoader.ClientConfig()
	if err != nil {
		//The stub is to allow commands that don't need a connection to a Kubernetes cluster.
		//We'll produce a warning and the stub itself will error when a command is trying to use it.
		panic(s.err())
	}

	s.client, err = client.New(cfg, client.Options{})
	if err != nil {
		panic(s.err())
	}
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

func NewAsBackplaneClusterAdmin(clusterID string, options client.Options) (client.Client, error) {
	bp, err := bpconfig.GetBackplaneConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to load backplane-cli config: %v", err)
	}

	cfg, err := bplogin.GetRestConfigAsUser(bp, clusterID, "backplane-cluster-admin")
	if err != nil {
		return nil, err
	}

	return client.New(cfg, options)
}
