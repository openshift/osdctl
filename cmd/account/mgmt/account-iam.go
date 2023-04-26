package mgmt

import (
	"fmt"
	"os"

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

func checkError(msg string, err error) {
	if err != nil {
		r := fmt.Errorf(msg, err)
		panic(r)
	}
}

func writeFile(user string, content string) {
	data := []byte(content)
	fileName := fmt.Sprintf("/tmp/aws-iam-%s", user)
	err := os.WriteFile(fileName, data, 0644)
	checkError("Failed to create credential file in /tmp", err)
	fmt.Printf("AWS credentials file created in %s\n", fileName)
}

func (o *iamOptions) run() error {
	// Build the base AWS client using the provide credentials (profile or env vars)
	awsClient, err := awsprovider.NewAwsClient(o.awsProfile, o.awsRegion, "")
	checkError("Could not build AWS Client: %s", err)

	// Get the right partition for the final ARN
	partition, err := awsprovider.GetAwsPartition(awsClient)
	checkError("Could not get AWS partition: %s", err)

	// Generate a session name using the SRE's kerberos ID
	sessionName, err := osdCloud.GenerateRoleSessionName(awsClient)
	checkError("Could not generate Session Name: %s", err)

	// Use OrganizationAccountAccessRole
	assumedRoleCreds, err := osdCloud.GenerateOrganizationAccountAccessCredentials(awsClient, o.awsAccountID, sessionName, partition)
	checkError("Could not build AWS Client for OrganizationAccountAccessRole: %s", err)

	// Variable with credential to be used by the impersonate aws client
	impersonateAwsCredentials := awsprovider.AwsClientInput{
		AccessKeyID:     *assumedRoleCreds.AccessKeyId,
		SecretAccessKey: *assumedRoleCreds.SecretAccessKey,
		SessionToken:    *assumedRoleCreds.SessionToken,
		Region:          o.awsRegion,
	}

	// Create a new impersonated AWS client
	impersonateAwsClient, err := awsprovider.NewAwsClientWithInput(&impersonateAwsCredentials)
	checkError("Could create impersonated AWS Client: %s", err)

	// Check if IAM user already exists
	iamUserExist, err := awsprovider.CheckIAMUserExists(impersonateAwsClient, &o.kerberosUser)
	if !iamUserExist {
		// Create IAM user
		err := awsprovider.CreateIAMUserAndAttachPolicy(impersonateAwsClient, &o.kerberosUser, &arnPolicy)
		checkError("Error Creating the IAM user: %s", err)

		// Create AccessKey
		newAccessKey, err := impersonateAwsClient.CreateAccessKey(&iam.CreateAccessKeyInput{UserName: &o.kerberosUser})
		checkError("Error creating the use Access Key: %s", err)

		// Creates the file with aws IAM user credentials
		content := fmt.Sprintf("[%v-dev-account]\naws_access_key_id = %v\naws_secret_access_key = %v\n", *newAccessKey.AccessKey.UserName, *newAccessKey.AccessKey.AccessKeyId, *newAccessKey.AccessKey.SecretAccessKey)
		writeFile(*newAccessKey.AccessKey.UserName, content)

		return nil

	} else {
		if o.rotate {
			existingKeys, err := impersonateAwsClient.ListAccessKeys(&iam.ListAccessKeysInput{UserName: &o.kerberosUser})
			checkError("Error getting existing access keys: %s", err)

			for i := range existingKeys.AccessKeyMetadata {
				_, err := impersonateAwsClient.DeleteAccessKey(&iam.DeleteAccessKeyInput{AccessKeyId: existingKeys.AccessKeyMetadata[i].AccessKeyId, UserName: &o.kerberosUser})
				checkError("Error deleting existing access keys: %s", err)
			}
			// Create AccessKey
			newAccessKey, err := impersonateAwsClient.CreateAccessKey(&iam.CreateAccessKeyInput{UserName: &o.kerberosUser})
			checkError("Error creating the use Access Key: %s", err)

			// Creates the file with aws IAM user credentials
			content := fmt.Sprintf("[%v-dev-account]\naws_access_key_id = %v\naws_secret_access_key = %v\n", *newAccessKey.AccessKey.UserName, *newAccessKey.AccessKey.AccessKeyId, *newAccessKey.AccessKey.SecretAccessKey)
			writeFile(*newAccessKey.AccessKey.UserName, content)

			return nil
		}

		// Print message if user exists and rotate not defined
		fmt.Printf("User %s already exists in AWS account %s\n", o.kerberosUser, o.awsAccountID)
		return err
	}

}
