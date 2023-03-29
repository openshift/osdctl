package osdCloud

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	compute "cloud.google.com/go/compute/apiv1"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"google.golang.org/api/iterator"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
)

type GcpProjectClaimSpec struct {
	GcpProjectID string `json:"gcpProjectID"`
}
type GcpProjectClaim struct {
	Spec GcpProjectClaimSpec `json:"spec"`
}

func ParseGcpProjectClaim(raw string) (*GcpProjectClaim, error) {
	var projectClaim GcpProjectClaim
	err := json.Unmarshal([]byte(raw), &projectClaim)
	if err != nil {
		return nil, err
	}
	return &projectClaim, nil
}

func GenerateGCPComputeInstancesClient() (*compute.InstancesClient, error) {
	ctx := context.Background()
	client, err := compute.NewInstancesRESTClient(ctx)
	return client, err
}

func ListInstances(client *compute.InstancesClient, projectID, zone string) *compute.InstanceIterator {
	ctx := context.Background()
	request := &computepb.ListInstancesRequest{
		Project: projectID,
		Zone:    zone,
	}
	return client.List(ctx, request)
}

// Concrete struct with fields required only for interacting with the GCP cloud.
type GcpCluster struct {
	*BaseClient
	ComputeClient *compute.InstancesClient
	ProjectId     string
	Zones         []string
}

func NewGcpCluster(ocmClient *sdk.Connection, clusterId string) (ClusterHealthClient, error) {
	clusterResp, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(clusterId).Get().Send()
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	cluster := clusterResp.Body()
	return &GcpCluster{
		BaseClient: &BaseClient{
			ClusterId: clusterId,
			OcmClient: ocmClient,
			Cluster:   cluster,
		},
	}, nil
}

func (g *GcpCluster) Login() error {
	clusterResources, err := g.OcmClient.ClustersMgmt().V1().Clusters().Cluster(g.ClusterId).Resources().Live().Get().Send()
	if err != nil {
		return err
	}
	projectClaimRaw, found := clusterResources.Body().Resources()["gcp_project_claim"]
	if !found {
		return fmt.Errorf("The gcp_project_claim was not found in the ocm resource")
	}
	projectClaim, err := ParseGcpProjectClaim(projectClaimRaw)
	if err != nil {
		log.Printf("Unmarshalling GCP projectClaim failed: %v\n", err)
		return err
	}
	g.ProjectId = projectClaim.Spec.GcpProjectID
	g.Zones = g.Cluster.Nodes().AvailabilityZones()
	if g.ProjectId == "" || len(g.Zones) == 0 {
		return fmt.Errorf("ProjectID or Zones empty - aborting")
	}
	g.ComputeClient, err = GenerateGCPComputeInstancesClient()
	if err != nil {
		return err
	}
	return nil
}

func (g *GcpCluster) Close() {
	if g.ComputeClient != nil {
		g.ComputeClient.Close()
	}
}

func (g *GcpCluster) GetAZs() []string {
	return g.Zones
}

func (g *GcpCluster) GetAllVirtualMachines(region string) ([]VirtualMachine, error) {
	vms := make([]VirtualMachine, 5)
	instances := ListInstances(g.ComputeClient, g.ProjectId, region)
	for {
		instance, err := instances.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		vm := VirtualMachine{
			Original: instance,
			Name:     instance.GetName(),
			Size:     instance.GetMachineType(),
			State:    strings.ToLower(instance.GetStatus()),
			Labels:   instance.GetLabels(),
		}
		vms = append(vms, vm)
	}
	return vms, nil
}
