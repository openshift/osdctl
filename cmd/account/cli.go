package account

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// newCmdCli implements the Cli command which generates temporary STS cli credentials for the specified account cr
func newCmdCli() *cobra.Command {
	ops := &cliOptions{}
	cliCmd := &cobra.Command{
		Use:               "cli",
		Short:             "Generate temporary AWS CLI credentials on demand",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
	}

	cliCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")
	cliCmd.Flags().StringVarP(&ops.awsAccountID, "accountId", "i", "", "AWS Account ID")
	cliCmd.Flags().StringVarP(&ops.awsProfile, "profile", "p", "", "AWS Profile")
	cliCmd.Flags().StringVarP(&ops.output, "output", "o", "", "Output type")
	cliCmd.Flags().StringVarP(&ops.region, "region", "r", "", "Region")
	cliCmd.Flags().StringVarP(&ops.clusterID, "clusterID", "C", "", "Cluster ID")

	return cliCmd
}

// cliOptions defines the struct for running the cli command
type cliOptions struct {
	output  string
	verbose bool

	awsAccountID string
	awsProfile   string
	region       string
	clusterID    string
}

func (o *cliOptions) complete(cmd *cobra.Command) error {

	var err error

	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocmClient.Close()

	if o.awsAccountID == "" && o.clusterID == "" {
		return fmt.Errorf("please specify -i or -C")
	}

	if o.awsAccountID != "" && o.clusterID != "" {
		return fmt.Errorf("-i and -c are mutually exclusive, please only specify one")
	}

	if o.clusterID != "" {
		o.awsAccountID, err = utils.GetAWSAccountIdForCluster(ocmClient, o.clusterID)
		if err != nil {
			return err
		}
	}

	if o.region == "" {
		o.region = "us-east-1"
	}

	return nil
}

func (o *cliOptions) run() error {

	var err error
	isCCS := false

	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocmClient.Close()

	// If a cluster ID was provided, determine if the cluster is CCS
	if o.clusterID != "" {
		isCCS, err = utils.IsClusterCCS(ocmClient, o.clusterID)
		if err != nil {
			return err
		}
	}

	// Build the base AWS client using the provide credentials (profile or env vars)
	awsClient, err := aws.NewAwsClient(o.awsProfile, o.region, "")
	if err != nil {
		fmt.Printf("Could not build AWS Client: %s\n", err)
		return err
	}

	// Get the right partition for the final ARN
	partition, err := aws.GetAwsPartition(awsClient)
	if err != nil {
		return err
	}

	// Generate a session name using the SRE's kerberos ID
	sessionName, err := osdCloud.GenerateRoleSessionName(awsClient)
	if err != nil {
		fmt.Printf("Could not generate Session Name: %s\n", err)
		return err
	}

	var assumedRoleCreds *types.Credentials
	if isCCS {
		// If the cluster is CCS, the target role needs to be determined, and the jump role chain needs to be executed

		// Determine the right jump role
		targetRoleArnString, err := utils.GetSupportRoleArnForCluster(ocmClient, o.clusterID)
		if err != nil {
			return err
		}

		targetRoleArn, err := arn.Parse(targetRoleArnString)
		if err != nil {
			return err
		}

		targetRoleArn.Partition = partition

		// Start the jump role chain. Result should be credentials for the ManagedOpenShift Support role for the target cluster
		assumedRoleCreds, err = osdCloud.GenerateSupportRoleCredentials(awsClient, o.region, sessionName, targetRoleArn.String())
		if err != nil {
			return err
		}

	} else {

		// If the cluster is non-CCS, or an AWS Account ID was provided with -i, try and use OrganizationAccountAccessRole
		assumedRoleCreds, err = osdCloud.GenerateOrganizationAccountAccessCredentials(awsClient, o.awsAccountID, sessionName, partition)
		if err != nil {
			fmt.Printf("Could not build AWS Client for OrganizationAccountAccessRole: %s\n", err)
			return err
		}
	}

	// Output section
	if o.output == "" {
		fmt.Printf("Temporary AWS Credentials:\n%v\n", assumedRoleCreds)
	}

	if o.output == "json" {
		fmt.Printf("{\n\"AccessKeyId\": %q, \n\"Expiration\": %q, \n\"SecretAccessKey\": %q, \n\"SessionToken\": %q, \n\"Region\": %q\n}",
			*assumedRoleCreds.AccessKeyId,
			*assumedRoleCreds.Expiration,
			*assumedRoleCreds.SecretAccessKey,
			*assumedRoleCreds.SessionToken,
			o.region,
		)
	}

	if o.output == "env" {
		fmt.Printf("AWS_ACCESS_KEY_ID=%s\nAWS_SECRET_ACCESS_KEY=%s\nAWS_SESSION_TOKEN=%s\nAWS_DEFAULT_REGION=%s\nAWS_REGION=%s\n",
			*assumedRoleCreds.AccessKeyId,
			*assumedRoleCreds.SecretAccessKey,
			*assumedRoleCreds.SessionToken,
			o.region,
			o.region,
		)
	}

	return nil
}
