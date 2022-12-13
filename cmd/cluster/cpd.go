package cluster

import (
	"context"
	"fmt"
	"os"

	awsSdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	onvUtils "github.com/openshift/osd-network-verifier/cmd/utils"
	"github.com/openshift/osd-network-verifier/pkg/verifier"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
)

type cpdOptions struct {
	clusterID  string
	awsProfile string
}

const (
	unknownProvisionCode = "OCM3999"
	cpdLongDescription   = `
Helps investigate OSD/ROSA cluster provisioning delays (CPD) or failures

  This command only supports AWS at the moment and will:
	
  * Check the cluster's dnszone.hive.openshift.io custom resource
  * Check whether a known OCM error code and message has been shared with the customer already
  * Check that the cluster's VPC and/or subnet route table(s) contain a route for 0.0.0.0/0 if it's BYOVPC
`
	cpdExample = `
  # Investigate a CPD for a cluster using an AWS profile named "rhcontrol"
  osdctl cluster cpd --cluster-id 1kfmyclusteristhebesteverp8m --profile rhcontrol
`
)

func newCmdCpd() *cobra.Command {
	ops := cpdOptions{}
	cpdCmd := &cobra.Command{
		Use:               "cpd",
		Short:             "Runs diagnostic for a Cluster Provisioning Delay (CPD)",
		Long:              cpdLongDescription,
		Example:           cpdExample,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {

			cmdutil.CheckErr(ops.run())
		},
	}
	cpdCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", ops.clusterID, "The internal/external (OCM) Cluster ID")
	cpdCmd.Flags().StringVarP(&ops.awsProfile, "profile", "p", ops.awsProfile, "AWS profile name")

	return cpdCmd
}

func (o *cpdOptions) run() error {
	// Get the cluster info
	ocmClient := utils.CreateConnection()
	defer ocmClient.Close()

	cluster, err := utils.GetClusterAnyStatus(ocmClient, o.clusterID)
	if err != nil {
		return err
	}

	fmt.Println("Checking if cluster has become ready")
	if cluster.Status().State() == "ready" {
		fmt.Printf("This cluster is in a ready state and already provisioned")
		return nil
	}

	fmt.Println("Checking if cluster DNS is ready")
	// Check if DNS is ready, exit out if not
	if !cluster.Status().DNSReady() {
		fmt.Println("DNS not ready. Investigate reasons using the dnszones CR in the cluster namespace:")
		fmt.Printf("oc get dnszones -n uhc-production-%s -o yaml --as backplane-cluster-admin\n", o.clusterID)
		return nil
	}

	fmt.Println("Checking if OCM error code is already known")
	// Check if the OCM Error code is a known error
	if cluster.Status().ProvisionErrorCode() != unknownProvisionCode {
		fmt.Printf("Error code %s is known, customer already received Service Log\n", cluster.Status().ProvisionErrorCode())
	}

	fmt.Println("Checking if cluster is GCP")
	// If the cluster is GCP, give instructions on how to get console access
	if cluster.CloudProvider().ID() == "gcp" {
		return fmt.Errorf("this command doesn't support GCP yet. Needs manual investigation:\nocm backplane cloud console -b %s", o.clusterID)
	}

	fmt.Println("Generating AWS credentials for cluster")
	// Get AWS credentials for the cluster
	awsClient, err := osdCloud.GenerateAWSClientForCluster(o.awsProfile, o.clusterID)
	if err != nil {
		fmt.Println("PLEASE CONFIRM YOUR CREDENTIALS ARE CORRECT. If you're absolutely sure they are, send this Service Log https://github.com/openshift/managed-notifications/blob/master/osd/aws/ROSA_AWS_invalid_permissions.json")
		fmt.Println(err)
		return err
	}

	// If the cluster is BYOVPC, check the route tables
	// This check is copied from ocm-cli
	if cluster.AWS().SubnetIDs() != nil && len(cluster.AWS().SubnetIDs()) > 0 {
		fmt.Println("Checking BYOVPC to ensure subnets have valid routing")
		for _, subnet := range cluster.AWS().SubnetIDs() {
			isValid, err := isSubnetRouteValid(awsClient, subnet)
			if err != nil {
				return err
			}
			if !isValid {
				return fmt.Errorf("subnet %s does not have a default route to 0.0.0.0/0\n Run the following to send a SerivceLog:\n osdctl servicelog post %s -t https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/aws/InstallFailed_NoRouteToInternet.json", subnet, o.clusterID)
			}
			// running the network verifier is the next step anyways, so let's save some keystrokes
			vei := verifier.ValidateEgressInput{
				Ctx:          context.TODO(),
				SubnetID:     subnet,
				InstanceType: "t3.micro",
			}

			awsVerifier, err := onvUtils.GetAwsVerifier(cluster.Region().DisplayName(), o.awsProfile, false)
			if err != nil {
				fmt.Printf("could not build awsVerifier %v", err)
				os.Exit(1)
			}

			output := verifier.ValidateEgress(awsVerifier, vei)
			_, _, errors := output.Parse()
			for _, err := range errors {
				return err
			}
		}
		return nil
	}

	fmt.Println("Next step: check the AWS resources manually, run ocm backplane cloud console")

	return nil
}

func isSubnetRouteValid(awsClient aws.Client, subnetID string) (bool, error) {
	var routeTable string

	// Try and find a Route Table associated with the given subnet
	describeRouteTablesOutput, err := awsClient.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   awsSdk.String("association.subnet-id"),
				Values: []*string{awsSdk.String(subnetID)},
			},
		},
	})
	if err != nil {
		return false, fmt.Errorf("failed to describe route tables associated to subnet %s: %w", subnetID, err)
	}

	// If there are no associated RouteTables, then the subnet uses the default RoutTable for the VPC
	if len(describeRouteTablesOutput.RouteTables) == 0 {
		// Get the VPC ID for the subnet
		describeSubnetOutput, err := awsClient.DescribeSubnets(&ec2.DescribeSubnetsInput{
			SubnetIds: []*string{&subnetID},
		})
		if err != nil {
			return false, err
		}
		if len(describeSubnetOutput.Subnets) == 0 {
			return false, fmt.Errorf("no subnets returned for subnet id %v", subnetID)
		}

		vpcID := *describeSubnetOutput.Subnets[0].VpcId

		// Set the route table to the default for the VPC
		routeTable, err = findDefaultRouteTableForVPC(awsClient, vpcID)
		if err != nil {
			return false, err
		}
	} else {
		// Set the route table to the one associated with the subnet
		routeTable = *describeRouteTablesOutput.RouteTables[0].RouteTableId
	}

	// Check that the RouteTable for the subnet has a default route to 0.0.0.0/0
	describeRouteTablesOutput, err = awsClient.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		RouteTableIds: []*string{awsSdk.String(routeTable)},
	})
	if err != nil {
		return false, err
	}

	if len(describeRouteTablesOutput.RouteTables) == 0 {
		// Shouldn't happen
		return false, fmt.Errorf("no route tables found for route table id %v", routeTable)
	}

	for _, route := range describeRouteTablesOutput.RouteTables[0].Routes {
		// Some routes don't use CIDR blocks as targets, so this needs to be checked
		if route.DestinationCidrBlock != nil && *route.DestinationCidrBlock == "0.0.0.0/0" {
			return true, nil
		}
	}

	// We haven't found a default route to the internet, so this subnet has an invalid route table
	return false, nil
}

// findDefaultRouteTableForVPC returns the AWS Route Table ID of the VPC's default Route Table
func findDefaultRouteTableForVPC(awsClient aws.Client, vpcID string) (string, error) {
	describeRouteTablesOutput, err := awsClient.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   awsSdk.String("vpc-id"),
				Values: []*string{awsSdk.String(vpcID)},
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
