package network

import (
	"context"
	"fmt"
	"github.com/openshift/osd-network-verifier/pkg/data/cloud"
	"github.com/openshift/osd-network-verifier/pkg/verifier"
	"golang.org/x/oauth2/google"
)

func (e *EgressVerification) setupForGcp(ctx context.Context) (*google.Credentials, error) {
	// If the user does not provide a ClusterID, but provides the Platform as GCP, require explicit subnet-id
	if e.ClusterId == "" {
		if e.SubnetIds == nil || e.platformName == "" {
			return nil, fmt.Errorf("--subnet-id and --platform are required when --cluster-id is not specified")
		}
	}

	return google.FindDefaultCredentials(ctx)
}

func (e *EgressVerification) generateGcpValidateEgressInput(ctx context.Context, platform cloud.Platform) ([]*verifier.ValidateEgressInput, error) {
	input, err := e.defaultValidateEgressInput(ctx, platform)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble validate egress input: %s", err)
	}

	// We don't store any tags for GCP in OCM like we do for AWS, so we just use the default
	// GCP doesn't support uppercase labels
	input.Tags["name"] = "osd-network-verifier"

	subnetIds, err := e.getGcpSubnetIds(ctx)
	if err != nil {
		return nil, err
	}
	input.SubnetID = subnetIds[0]

	// Allow overriding the region
	if e.Region != "" {
		input.GCP.Region = e.Region
	} else {
		input.GCP.Region = e.cluster.Region().ID()
	}
	if input.GCP.Region == "" {
		return nil, fmt.Errorf("could not get region from OCM, specify manually with --region")
	}

	if e.VpcName != "" {
		input.GCP.VpcName = e.VpcName
	} else {
		input.GCP.VpcName = e.cluster.GCPNetwork().VPCName()
	}
	if input.GCP.VpcName == "" {
		return nil, fmt.Errorf("could not get vpc name from OCM, specify manually with --vpc")
	}

	// Allow overriding the project ID
	if e.GcpProjectID != "" {
		input.GCP.ProjectID = e.GcpProjectID
	} else {
		input.GCP.ProjectID = e.cluster.GCP().ProjectID()
	}
	if input.GCP.ProjectID == "" {
		return nil, fmt.Errorf("could not get GCP project ID from OCM, specify manually with --gcp-project-id")
	}

	// We default to zone B for the region, as does osd-network-verifier, since it has the most instance types.
	input.GCP.Zone = fmt.Sprintf("%s-b", input.GCP.Region)

	// Creating a slice of input values for the network-verifier to loop over.
	// All inputs are essentially equivalent except their subnet ids
	inputs := make([]*verifier.ValidateEgressInput, len(subnetIds))
	for i := range subnetIds {
		// Copying a pointer to avoid overwriting it
		var myinput = &verifier.ValidateEgressInput{}
		*myinput = *input
		inputs[i] = myinput
		inputs[i].SubnetID = subnetIds[i]
	}

	return inputs, nil
}

func (e *EgressVerification) getGcpSubnetIds(ctx context.Context) ([]string, error) {
	if e.SubnetIds != nil {
		e.log.Info(ctx, "using manually specified subnet-id(s): %s", e.SubnetIds)
		return e.SubnetIds, nil
	}

	// TODO: Add support for non-BYOVPC in GCP
	// Protect against non-BYOVPC, where we don't have OCM data on subnets.
	if e.cluster.GCPNetwork().ComputeSubnet() == "" && e.cluster.GCPNetwork().ControlPlaneSubnet() == "" {
		return nil, fmt.Errorf("this cluster is non-BYOVPC and discovering subnets not yet supported, pass via --subnet-id")
	}

	subnetIds := []string{
		e.cluster.GCPNetwork().ComputeSubnet(),
		e.cluster.GCPNetwork().ControlPlaneSubnet(),
	}

	return subnetIds, nil
}
