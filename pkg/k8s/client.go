package k8s

import (
	"context"
	"fmt"
	"log"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type StubClient struct {
}

func (s *StubClient) err() error {
	return fmt.Errorf("Not connected to real cluster, please verify KUBECONFIG is correct")
}

func (s *StubClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	return s.err()
}

func (s *StubClient) List(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
	return s.err()
}

func (s *StubClient) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
	return s.err()
}

func (s *StubClient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
	return s.err()
}

func (s *StubClient) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	return s.err()
}

func (s *StubClient) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	return s.err()
}

func (s *StubClient) DeleteAllOf(ctx context.Context, obj runtime.Object, opts ...client.DeleteAllOfOption) error {
	return s.err()
}

func (s *StubClient) Status() client.StatusWriter {
	panic(s.err())
}

func NewClient(flags *genericclioptions.ConfigFlags) client.Client {
	configLoader := flags.ToRawKubeConfigLoader()
	cfg, err := configLoader.ClientConfig()
	if err != nil {
		//The stub is to allow commands that don't need a connection to a Kubernetes cluster.
		//We'll produce a warning and the stub itself will error when a command is trying to use it.
		log.Printf("Can't load KubeConfig, using stub client: %v", err)
		return &StubClient{}
	}

	cli, err := client.New(cfg, client.Options{})
	if err != nil {
		log.Printf("Can't load KubeConfig, using stub client: %v", err)
		return &StubClient{}
	}
	return cli
}
