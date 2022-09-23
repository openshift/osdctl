package k8s

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LazyClient struct {
	client client.Client
	flags  *genericclioptions.ConfigFlags
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

func (s *LazyClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
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
