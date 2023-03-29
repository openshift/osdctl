package osdCloud

import (
	sdk "github.com/openshift-online/ocm-sdk-go"
	ocmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

// This client is used to interface with AWS & GCP and provide common
// abstractions that are generated from the cloud-specific resources.
// Right now the client is only used by the `osdctl cluster health` command and only
// provides functions used in that command.
// It can and should be extended as seen fit if it seems useful.
type ClusterHealthClient interface {
	Login() error
	GetCluster() *ocmv1.Cluster
	GetAZs() []string
	GetAllVirtualMachines(region string) ([]VirtualMachine, error)
	Close()
}

// A common struct used to not repeat fields used in the sub'classes' for AWS and GCP.
type BaseClient struct {
	ClusterId string
	OcmClient *sdk.Connection
	Cluster   *ocmv1.Cluster
}

func (b *BaseClient) GetCluster() *ocmv1.Cluster {
	return b.Cluster
}

// Abstract the AWS instances and GCP instances into a common type.
// The Original field should store the data returned by the cloud directly, so it can be accessed via casting if needed.
type VirtualMachine struct {
	Original interface{}
	Name     string
	Size     string
	State    string
	Labels   map[string]string
}
