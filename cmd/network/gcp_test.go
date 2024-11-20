package network

import (
	"context"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osd-network-verifier/pkg/data/cloud"
	"testing"
)

func Test_setupForGcp_InvalidOptions(t *testing.T) {
	e := &EgressVerification{}
	_, err := e.setupForGcp(context.Background())
	if err != nil && err.Error() != "--subnet-id and --platform are required when --cluster-id is not specified" {
		t.Errorf("expected error but got %s", err)
	}
}

// This tests GCP with data being pulled from OCM. We hydrate a cluster with the data versus mocking OCM
func Test_generateGcpValidateEgressInput_DefaultBehavior(t *testing.T) {
	e := &EgressVerification{
		cluster: newTestCluster(t, cmv1.NewCluster().
			CloudProvider(cmv1.NewCloudProvider().ID("gcp")).
			Region(cmv1.NewCloudRegion().ID("us-east1")).
			Product(cmv1.NewProduct().ID("rosa")).
			GCP(cmv1.NewGCP().ProjectID("my-project")).
			GCPNetwork(cmv1.NewGCPNetwork().VPCName("test-vpc").ComputeSubnet("subnet-compute").ControlPlaneSubnet("subnet-cpane")),
		),
		log: newTestLogger(t),
	}
	inputs, err := e.generateGcpValidateEgressInput(context.Background(), cloud.GCPClassic)
	if err != nil {
		t.Errorf("expected no error but got %s", err)
	}

	if len(inputs) != 2 {
		t.Errorf("expected 2 inputs but got %d", len(inputs))
	}

	sampleInput := inputs[0]
	if sampleInput.SubnetID != "subnet-compute" {
		t.Errorf("expected subnet-compute but got %s", inputs[0].SubnetID)
	}

	if sampleInput.GCP.Zone != "us-east1-b" {
		t.Errorf("expected GCP.Zone to be us-east1-b but got %s", sampleInput.GCP.Zone)
	}

	if sampleInput.Tags["name"] != "osd-network-verifier" {
		t.Errorf("expected tag name=osd-network-verifier but got value %s", sampleInput.Tags["name"])
	}
}

// This tests GCP without data from OCM
func Test_generateGcpValidateEgressInput_UserProvidedValues(t *testing.T) {
	e := &EgressVerification{
		Region:       "us-east1",
		GcpProjectID: "my-project",
		SubnetIds:    []string{"subnet-compute", "subnet-cpane"},
		VpcName:      "test-vpc",
		log:          newTestLogger(t),
	}
	inputs, err := e.generateGcpValidateEgressInput(context.Background(), cloud.GCPClassic)
	if err != nil {
		t.Errorf("expected no error but got %s", err)
	}

	sampleInput := inputs[0]
	if sampleInput.SubnetID != "subnet-compute" {
		t.Errorf("expected subnet-compute but got %s", inputs[0].SubnetID)
	}

	if sampleInput.GCP.Zone != "us-east1-b" {
		t.Errorf("expected GCP.Zone to be us-east1-b but got %s", sampleInput.GCP.Zone)
	}

	if sampleInput.Tags["name"] != "osd-network-verifier" {
		t.Errorf("expected tag name=osd-network-verifier but got value %s", sampleInput.Tags["name"])
	}
}
