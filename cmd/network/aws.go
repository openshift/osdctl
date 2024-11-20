package network

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/openshift/osd-network-verifier/pkg/data/cloud"
	"github.com/openshift/osd-network-verifier/pkg/verifier"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"strings"
)

type egressVerificationAWSClient interface {
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
	DescribeRouteTables(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
}

// setupForAws configures an EgressVerification's awsClient and cluster depending on whether the ClusterId or profile
// flags are supplied. It also returns an aws.Config if needed.
func (e *EgressVerification) setupForAws(ctx context.Context) (*aws.Config, error) {
	// If ClusterId is supplied, leverage ocm and ocm-backplane to get an AWS client.
	// We previously hydrated the EgressVerification struct with a `cluster` in this scenario.
	if e.ClusterId != "" && e.cluster != nil {
		ocmClient, err := utils.CreateConnection()
		if err != nil {
			return nil, fmt.Errorf("error creating OCM connection: %v", err)
		}
		defer ocmClient.Close()

		// We currently have insufficient permissions to run network verifier on ROSA HCP
		// We can update or, if applicable, remove this warning after https://issues.redhat.com/browse/XCMSTRAT-245
		if e.cluster.Hypershift().Enabled() {
			e.log.Warn(ctx, "Generally, SRE has insufficient AWS permissions"+
				" to run network verifier on hosted control plane clusters. Run anyway?")
			if !utils.ConfirmPrompt() {
				return nil, errors.New("You can try the network verifier script in ops-sop/hypershift/utils/verify-egress.sh")
			}
		}

		e.log.Info(ctx, "getting AWS credentials from backplane-api")
		cfg, err := osdCloud.CreateAWSV2Config(ocmClient, e.cluster)
		if err != nil {
			return nil, fmt.Errorf("failed to get credentials automatically from backplane-api: %v."+
				" You can still try this command by exporting AWS credentials as environment variables and specifying"+
				" the --subnet-id, --security-group, and other required flags manually."+
				" See osdctl network verify-egress -h for more details", err)
		}
		e.log.Debug(ctx, "retrieved AWS credentials from backplane-api")
		e.awsClient = ec2.NewFromConfig(cfg)
		return &cfg, nil
	}

	if e.SubnetIds == nil || e.SecurityGroupId == "" || e.platformName == "" {
		return nil, fmt.Errorf("--subnet-id, --security-group, and --platform are required when --cluster-id is not specified")
	}

	e.log.Info(ctx, "[WARNING] no cluster-id specified, there is reduced validation around the security group, subnet, and proxy, causing inaccurate results")
	e.log.Info(ctx, "using whatever default AWS credentials are locally available")
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("network verification failed to find valid creds locally: %s", err)
	}

	// Additionally, if an AWS region must be provided somehow if there's no ClusterId
	// This could have been done via the default AWS credentials or can be supplied manually via --region
	if e.Region != "" {
		e.log.Info(ctx, "overriding region with %s", e.Region)
		cfg.Region = e.Region
	}

	e.awsClient = ec2.NewFromConfig(cfg)

	return &cfg, nil
}

// generateAWSValidateEgressInput is an opinionated interface in front of osd-network-verifier.
// Its input is an OCM internal/external ClusterId, and it returns the corresponding input to osd-network-verifier with
// default AWS tags, one of the cluster's private subnet IDs, and the cluster's master security group.
// Can override SecurityGroupId and SubnetIds.
func (e *EgressVerification) generateAWSValidateEgressInput(ctx context.Context, platform cloud.Platform) ([]*verifier.ValidateEgressInput, error) {
	input, err := e.defaultValidateEgressInput(ctx, platform)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble validate egress input: %s", err)
	}

	// Append any tags from OCM for this cluster
	for k, v := range e.cluster.AWS().Tags() {
		input.Tags[k] = v
	}
	input.Tags["Name"] = "osd-network-verifier"

	if e.cluster != nil {
		// If a KMS key is defined for the cluster, use it as the default aws/ebs key may not exist
		clusterAWS := e.cluster.AWS()

		if clusterAWS != nil {
			if kmsKeyArn, isOk := clusterAWS.GetKMSKeyArn(); isOk {
				e.log.Info(ctx, "using KMS key defined for the cluster: %s", kmsKeyArn)
				input.AWS.KmsKeyID = kmsKeyArn
			}
		}
	}

	// Obtain subnet ids to run against
	subnetId, err := e.getAwsSubnetIds(ctx)
	if err != nil {
		return nil, err
	}

	input.SubnetID = subnetId[0]

	checkPublic, err := e.isSubnetPublic(ctx, input.SubnetID)
	if err != nil {
		return nil, err
	}
	if checkPublic {
		return nil, fmt.Errorf("subnet %v you provided is public. The network verifier only works for private subnets. Please provide a private subnet ID", input.SubnetID)
	}

	// Obtain security group id to use
	sgId, err := e.getSecurityGroupId(ctx)
	if err != nil {
		return nil, err
	}
	input.AWS.SecurityGroupIDs = []string{sgId}

	// Creating a slice of input values for the network-verifier to loop over.
	// All inputs are essentially equivalent except their subnet ids
	inputs := make([]*verifier.ValidateEgressInput, len(subnetId))
	for i := range subnetId {
		// Copying a pointer to avoid overwriting it
		var myinput = &verifier.ValidateEgressInput{}
		*myinput = *input
		inputs[i] = myinput
		inputs[i].SubnetID = subnetId[i]
	}

	return inputs, nil
}

// getAwsSubnetIds attempts to return a private subnet ID or all private subnet IDs of the cluster.
// e.SubnetIds acts as an override, otherwise e.awsClient will be used to attempt to determine the correct subnets
func (e *EgressVerification) getAwsSubnetIds(ctx context.Context) ([]string, error) {
	// A SubnetIds was manually specified, just use that
	if e.SubnetIds != nil {
		e.log.Info(ctx, "using manually specified subnet-id(s): %s", e.SubnetIds)
		return e.SubnetIds, nil
	}

	// If this is a non-BYOVPC cluster, we can find the private subnets based on the cluster and internal-elb tag
	if len(e.cluster.AWS().SubnetIDs()) == 0 {
		e.log.Info(ctx, "searching for subnets by tags: kubernetes.io/cluster/%s and %s=", e.cluster.InfraID(), nonByovpcPrivateSubnetTagKey)
		resp, err := e.awsClient.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
			Filters: []types.Filter{
				{
					Name:   aws.String("tag-key"),
					Values: []string{fmt.Sprintf("kubernetes.io/cluster/%s", e.cluster.InfraID())},
				},
				{
					Name:   aws.String("tag-key"),
					Values: []string{nonByovpcPrivateSubnetTagKey},
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to find private subnets for %s: %w", e.cluster.InfraID(), err)
		}

		if len(resp.Subnets) == 0 {
			return nil, fmt.Errorf("found 0 subnets with tags: kubernetes.io/cluster/%s and %s, consider the --subnet-id flag", e.cluster.InfraID(), nonByovpcPrivateSubnetTagKey)
		}
		if e.AllSubnets {
			subnets := make([]string, len(resp.Subnets))
			e.log.Debug(ctx, "Found %v subnets.", len(resp.Subnets))
			for i := range resp.Subnets {
				e.log.Debug(ctx, "Found subnet: %v", *resp.Subnets[i].SubnetId)
				subnets[i] = *resp.Subnets[i].SubnetId
			}
			return subnets, nil
		} else {
			e.log.Info(ctx, "using subnet-id: %s", *resp.Subnets[0].SubnetId)
			return []string{*resp.Subnets[0].SubnetId}, nil
		}
	}

	// For PrivateLink clusters, any provided subnet is considered a private subnet
	if e.cluster.AWS().PrivateLink() {
		if len(e.cluster.AWS().SubnetIDs()) == 0 {
			return nil, fmt.Errorf("unexpected error: %s is a PrivateLink cluster, but no subnets in OCM", e.cluster.InfraID())
		}
		// If the all-subnets flag is on, the network verifier will iterate over all subnets listed by ocm
		if e.AllSubnets {

			subnets := e.cluster.AWS().SubnetIDs()

			e.log.Debug(ctx, "Found the following subnets listed with ocm: %v", subnets)
			e.log.Debug(ctx, "Assigned value to var e.SubnetIds: %v", subnets)
			return subnets, nil

		}

		e.log.Info(ctx, "detected BYOVPC PrivateLink cluster, using first subnet from OCM: %s", e.cluster.AWS().SubnetIDs()[0])
		return []string{e.cluster.AWS().SubnetIDs()[0]}, nil

	}

	// For non-PrivateLink BYOVPC clusters, provided subnets are 50/50 public/private subnets, so make the user decide for now
	// TODO: Figure out via IGW/NAT GW/Route Tables
	return nil, fmt.Errorf("unable to determine which non-PrivateLink BYOVPC subnets are private yet, please check manually and provide the --subnet-id flag")

}

// This function checks the gateway attached to the subnet and returns true if the subnet starts with igw- (for InternetGateway) and has a route to 0.0.0.0/0
func (e *EgressVerification) isSubnetPublic(ctx context.Context, subnetID string) (bool, error) {
	var routeTable string

	// Try and find a Route Table associated with the given subnet

	routeTable, err := utils.FindRouteTableForSubnetForVerification(e.awsClient, subnetID)

	// Check that the RouteTable for the subnet has a default route to 0.0.0.0/0
	describeRouteTablesOutput, err := e.awsClient.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		RouteTableIds: []string{routeTable},
	})
	if err != nil {
		return false, err
	}

	if len(describeRouteTablesOutput.RouteTables) == 0 {
		// Shouldn't happen
		return false, fmt.Errorf("no route tables found for route table id %v", routeTable)
	}

	// Checking if the attached gateway starts with igw- for Internet Gateway
	for _, route := range describeRouteTablesOutput.RouteTables[0].Routes {
		if route.GatewayId != nil && strings.HasPrefix(*route.GatewayId, "igw-") {
			// Some routes don't use CIDR blocks as targets, so this needs to be checked
			if route.DestinationCidrBlock != nil && *route.DestinationCidrBlock == "0.0.0.0/0" {
				return true, nil
			}
		}
	}

	// We haven't found a default route to the internet, so this subnet can't be public
	return false, nil
}

// findDefaultRouteTableForVPC returns the AWS Route Table ID of the VPC's default Route Table
func (e *EgressVerification) findDefaultRouteTableForVPC(ctx context.Context, vpcID string) (string, error) {
	describeRouteTablesOutput, err := e.awsClient.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []string{vpcID},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to describe route tables associated with vpc %s: %w", vpcID, err)
	}

	for _, rt := range describeRouteTablesOutput.RouteTables {
		for _, assoc := range rt.Associations {
			if *assoc.Main {
				return *rt.RouteTableId, nil
			}
		}
	}

	return "", fmt.Errorf("no default route table found for vpc: %s", vpcID)
}

// getSecurityGroupId attempts to return a cluster's master node security group id
// e.SecurityGroupId acts as an override, otherwise e.awsClient will be used to attempt to determine the correct security group
func (e *EgressVerification) getSecurityGroupId(ctx context.Context) (string, error) {
	// A SecurityGroupId was manually specified, just use that
	if e.SecurityGroupId != "" {
		e.log.Info(ctx, "using manually specified security-group-id: %s", e.SecurityGroupId)
		return e.SecurityGroupId, nil
	}

	var filters []types.Filter
	platform, err := e.getPlatform()
	if err != nil {
		return "", err
	}
	switch platform {
	case cloud.AWSHCP, cloud.AWSHCPZeroEgress:
		filters = []types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{fmt.Sprintf("%s-default-sg", e.cluster.ID())},
			},
		}
	default:
		filters = []types.Filter{
			{
				// Prior to 4.16: <infra_id>-master-sg
				// 4.16+: <infra_id>-controlplane
				Name:   aws.String("tag:Name"),
				Values: []string{fmt.Sprintf("%s-master-sg", e.cluster.InfraID()), fmt.Sprintf("%s-controlplane", e.cluster.InfraID())},
			},
		}
	}

	// If no SecurityGroupId override is passed in, try to determine the master security group id
	e.log.Info(ctx, "searching for security group by tags: %s", filtersToString(filters))
	resp, err := e.awsClient.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: filters,
	})
	if err != nil {
		return "", fmt.Errorf("failed to find security group for %s: %w", e.cluster.InfraID(), err)
	}

	if len(resp.SecurityGroups) == 0 {
		return "", fmt.Errorf("failed to find any security groups by tags: %s", filtersToString(filters))
	}

	e.log.Info(ctx, "using security-group-id: %s", *resp.SecurityGroups[0].GroupId)
	return *resp.SecurityGroups[0].GroupId, nil
}
