package k8s

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewClient creates a Kubernetes client with provided cli flags
func NewClient(flags *genericclioptions.ConfigFlags) (client.Client, error) {
	configLoader := flags.ToRawKubeConfigLoader()
	cfg, err := configLoader.ClientConfig()
	if err != nil {
		return nil, err
	}

	cli, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, err
	}
	return cli, nil
}
