package cluster

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/backplane-cli/pkg/ocm"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/openshift/osdctl/cmd/network"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type cpdOptions struct {
	clusterID  string
	awsProfile string
}

const (
	cpdLongDescription = `
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
	OldFlowSupportRole = "role/RH-Technical-Support-Access"
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
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
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
		fmt.Printf("ocm-backplane elevate \"$(read -p 'Enter reason for elevation:' REASON && echo $REASON)\" -- get dnszones -n uhc-production-%s -o yaml\n", o.clusterID)
		return nil
	}

	fmt.Println("Checking if cluster is GCP")
	// If the cluster is GCP, give instructions on how to get console access
	if cluster.CloudProvider().ID() == "gcp" {
		return fmt.Errorf("this command doesn't support GCP yet. Needs manual investigation:\nocm backplane cloud console -b %s", o.clusterID)
	}

	awsv2cfg, err := osdCloud.CreateAWSV2Config(ocmClient, cluster)
	if err != nil {
		return fmt.Errorf("failed to build aws client config: %w\nManual investigation required", err)
	}
	creds, err := awsv2cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		return fmt.Errorf("failed to retrieve aws credentials: %w\nManual investigation required", err)
	}
	awsClient, err := aws.NewAwsClientWithInput(&aws.ClientInput{
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		Region:          cluster.Region().ID(),
	})
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
			fmt.Printf("subnet: %v\n", subnet)
			isValid, err := isSubnetRouteValid(awsClient, subnet)
			if err != nil {
				return err
			}
			if !isValid {
				return fmt.Errorf("subnet %s does not have a default route to 0.0.0.0/0\n Run the following to send a SerivceLog:\n osdctl servicelog post %s -t https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/aws/InstallFailed_NoRouteToInternet.json", subnet, o.clusterID)
			}
		}
		fmt.Printf("Attempting to run: osdctl network verify-egress --cluster-id %s\n", o.clusterID)
		ev := &network.EgressVerification{ClusterId: o.clusterID}
		ev.Run(context.Background())
		return nil
	}

	fmt.Println("Next step: check the AWS resources manually, run ocm backplane cloud console")

	return nil
}

func isSubnetRouteValid(awsClient aws.Client, subnetID string) (bool, error) {
	routeTable, err := utils.FindRouteTableForSubnet(awsClient, subnetID)
	if err != nil {
		return false, fmt.Errorf("failed to find routetable for subnet: %w", err)
	}

	// Check that the RouteTable for the subnet has a default route to 0.0.0.0/0
	describeRouteTablesOutput, err := awsClient.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		RouteTableIds: []string{routeTable},
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

func isIsolatedBackplaneAccess(cluster *cmv1.Cluster, ocmConnection *sdk.Connection) (bool, error) {
	if cluster.Hypershift().Enabled() {
		return true, nil
	}

	if cluster.AWS().STS().Enabled() {
		stsSupportJumpRole, err := ocm.DefaultOCMInterface.GetStsSupportJumpRoleARN(ocmConnection, cluster.ID())
		if err != nil {
			return false, fmt.Errorf("failed to get sts support jump role ARN for cluster %v: %w", cluster.ID(), err)
		}
		supportRoleArn, err := arn.Parse(stsSupportJumpRole)
		if err != nil {
			return false, fmt.Errorf("failed to parse ARN for jump role %v: %w", stsSupportJumpRole, err)
		}
		if supportRoleArn.Resource != OldFlowSupportRole {
			return true, nil
		}
	}

	return false, nil
}
