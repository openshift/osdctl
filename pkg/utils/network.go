package utils

import (
	"fmt"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/openshift/osdctl/pkg/provider/aws"
)

// Try and find a Route Table associated with the given subnet

func findRouteTableForSubnet(subnetID string, awsClient aws.Client) (string, error) {

	var routeTable string
	describeRouteTablesOutput, err := awsClient.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []types.Filter{
			{
				Name:   awsSdk.String("association.subnet-id"),
				Values: []string{subnetID},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to describe route tables associated to subnet %s: %w", subnetID, err)
	}

	// If there are no associated RouteTables, then the subnet uses the default RoutTable for the VPC
	if len(describeRouteTablesOutput.RouteTables) == 0 {
		// Get the VPC ID for the subnet
		describeSubnetOutput, err := awsClient.DescribeSubnets(&ec2.DescribeSubnetsInput{
			SubnetIds: []string{subnetID},
		})
		if err != nil {
			return "", err
		}
		if len(describeSubnetOutput.Subnets) == 0 {
			return "", fmt.Errorf("no subnets returned for subnet id %v", subnetID)
		}

		vpcID := *describeSubnetOutput.Subnets[0].VpcId

		// Set the route table to the default for the VPC
		routeTable, err = findDefaultRouteTableForVPC(awsClient, vpcID)
		if err != nil {
			return "", err
		}
	} else {
		// Set the route table to the one associated with the subnet
		routeTable = *describeRouteTablesOutput.RouteTables[0].RouteTableId
	}
	return routeTable, err
}

// findDefaultRouteTableForVPC returns the AWS Route Table ID of the VPC's default Route Table
func findDefaultRouteTableForVPC(awsClient aws.Client, vpcID string) (string, error) {
	describeRouteTablesOutput, err := awsClient.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []types.Filter{
			{
				Name:   awsSdk.String("vpc-id"),
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
