package mgmt

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	osdCloud "github.com/openshift/osdctl/pkg/osdCloud"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// iamOptions defines the struct for running the iam command
type iamOptions struct {
	awsAccountID string
	awsProfile   string
	awsRegion    string
	kerberosUser string
	rotate       bool
}

var arnPolicy = "arn:aws:iam::aws:policy/AdministratorAccess"

// accountIamCmd implements the accountIam command which creates an IAM user for a given account
func newCmdAccountIAM(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := &iamOptions{}
	iamCmd := &cobra.Command{
		Use:               "iam",
		Short:             "Creates an IAM user in a given AWS account and prints out the credentials",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	iamCmd.Flags().StringVarP(&ops.awsAccountID, "accountId", "i", "", "AWS account ID to run this against")
	iamCmd.Flags().StringVarP(&ops.kerberosUser, "user", "u", "", "Kerberos username to run this for")
	iamCmd.Flags().StringVarP(&ops.awsProfile, "profile", "p", "", "AWS Profile")
	iamCmd.Flags().StringVarP(&ops.awsRegion, "region", "r", "", "AWS Region")
	iamCmd.Flags().BoolVarP(&ops.rotate, "rotate", "R", false, "Rotate an IAM user's credentials and print the output")

	return iamCmd
}

func (o *iamOptions) complete(cmd *cobra.Command, args []string) error {
	if o.awsAccountID == "" {
		return cmdutil.UsageErrorf(cmd, "AWS Account ID must be provided")
	}
	if o.kerberosUser == "" {
		return cmdutil.UsageErrorf(cmd, "Kerberos UID is required")
	}
	if o.awsRegion == "" {
		o.awsRegion = "us-east-1"
	}
	if o.awsProfile == "" {
		o.awsProfile = "osd-staging-2"
	}
	return nil
}

func (o *iamOptions) run() error {
	// Build the base AWS client using the provide credentials (profile or env vars)
	awsClient, err := awsprovider.NewAwsClient(o.awsProfile, o.awsRegion, "")
	if err != nil {
		fmt.Printf("Could not build AWS Client: %s\n", err)
		return err
	}

	// Get the right partition for the final ARN
	partition, err := awsprovider.GetAwsPartition(awsClient)
	if err != nil {
		fmt.Printf("Could not get AWS partition: %s\n", err)
		return err
	}

	// Generate a session name using the SRE's kerberos ID
	sessionName, err := osdCloud.GenerateRoleSessionName(awsClient)
	if err != nil {
		fmt.Printf("Could not generate Session Name: %s\n", err)
		return err
	}
	// Use OrganizationAccountAccessRole
	assumedRoleCreds, err := osdCloud.GenerateOrganizationAccountAccessCredentials(awsClient, o.awsAccountID, sessionName, partition)
	if err != nil {
		fmt.Printf("Could not build AWS Client for OrganizationAccountAccessRole: %s\n", err)
		return err
	}
	// Variable with credential to be used by the impersonate aws client
	impersonateAwsCredentials := awsprovider.AwsClientInput{
		AccessKeyID:     *assumedRoleCreds.AccessKeyId,
		SecretAccessKey: *assumedRoleCreds.SecretAccessKey,
		SessionToken:    *assumedRoleCreds.SessionToken,
		Region:          o.awsRegion,
	}

	// Create a new impersonated AWS client
	impersonateAwsClient, err := awsprovider.NewAwsClientWithInput(&impersonateAwsCredentials)
	if err != nil {
		fmt.Printf("Could create impersonated AWS Client: %s\n", err)
		return err
	}

	// Check if IAM user already exists
	iamUserExist, err := awsprovider.CheckIAMUserExists(impersonateAwsClient, &o.kerberosUser)
	if !iamUserExist {
		// Create IAM user
		err := awsprovider.CreateIAMUserAndAttachPolicy(impersonateAwsClient, &o.kerberosUser, &arnPolicy)
		if err != nil {
			fmt.Printf("Error Creating the IAM user: %s\n", err)
			return err
		}
		// Create AccessKey
		newAccessKey, err := impersonateAwsClient.CreateAccessKey(&iam.CreateAccessKeyInput{UserName: &o.kerberosUser})
		if err != nil {
			fmt.Printf("Error creating the use Access Key: %s\n", err)
			return err
		}
		// Print the output of the
		fmt.Printf("Add the follow block to ~/.aws/credentials:\n[%v-dev-account]\naws_access_key_id = %v\naws_secret_access_key = %v\n", *newAccessKey.AccessKey.UserName, *newAccessKey.AccessKey.AccessKeyId, *newAccessKey.AccessKey.SecretAccessKey)
		return nil
	} else {
		if o.rotate {
			existingKeys, err := impersonateAwsClient.ListAccessKeys(&iam.ListAccessKeysInput{UserName: &o.kerberosUser})
			if err != nil {
				fmt.Printf("Error getting existing access keys: %s\n", err)
				return err
			}
			for i := range existingKeys.AccessKeyMetadata {
				_, err := impersonateAwsClient.DeleteAccessKey(&iam.DeleteAccessKeyInput{AccessKeyId: existingKeys.AccessKeyMetadata[i].AccessKeyId, UserName: &o.kerberosUser})
				if err != nil {
					fmt.Printf("Error deleting existing access keys: %s\n", err)
					return err
				}
			}
			// Create AccessKey
			newAccessKey, err := impersonateAwsClient.CreateAccessKey(&iam.CreateAccessKeyInput{UserName: &o.kerberosUser})
			if err != nil {
				fmt.Printf("Error creating the use Access Key: %s\n", err)
				return err
			}
			// Print the output of the
			fmt.Printf("Add the follow block to ~/.aws/credentials:\n[%v-dev-account]\naws_access_key_id = %v\naws_secret_access_key = %v\n", *newAccessKey.AccessKey.UserName, *newAccessKey.AccessKey.AccessKeyId, *newAccessKey.AccessKey.SecretAccessKey)
			return nil

		}
		fmt.Printf("User %s already exists in AWS account %s\n", o.kerberosUser, o.awsAccountID)
		return err
	}

}
