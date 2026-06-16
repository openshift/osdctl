package srelib

import (
	"fmt"
	"os/exec"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"

	"github.com/petrkotas/srelib/sdk"
	v1 "github.com/petrkotas/srelib/sdk/v1"
)

type Client struct {
	inner  v1.Client
	killer *plugin.Client
}

func NewClient(pluginPath string) (*Client, error) {
	pc := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: sdk.HandshakeConfig,
		VersionedPlugins: map[int]plugin.PluginSet{
			1: {"srelib": &v1.Plugin{}},
		},
		Cmd:    exec.Command(pluginPath),
		Logger: hclog.New(&hclog.LoggerOptions{Name: "srelib", Level: hclog.Error}),
	})

	rpcClient, err := pc.Client()
	if err != nil {
		pc.Kill()
		return nil, fmt.Errorf("srelib: connect to plugin: %w", err)
	}

	raw, err := rpcClient.Dispense("srelib")
	if err != nil {
		pc.Kill()
		return nil, fmt.Errorf("srelib: dispense plugin: %w", err)
	}

	return &Client{inner: raw.(v1.Client), killer: pc}, nil
}

func (c *Client) GetClusters(ids []string) ([]*cmv1.Cluster, error) {
	return c.inner.GetClusters(ids)
}

func (c *Client) GetClusterAnyStatus(id string) (*cmv1.Cluster, error) {
	return c.inner.GetClusterAnyStatus(id)
}

func (c *Client) Close() {
	c.killer.Kill()
}
