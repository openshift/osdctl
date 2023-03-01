package osdCloud

import (
	"context"
	"encoding/json"

	compute "cloud.google.com/go/compute/apiv1"
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
