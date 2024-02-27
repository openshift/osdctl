package mgmt

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/osdCloud"
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
func newCmdAccountIAM(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {
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
		fmt.Println("using default AWS Region: us-east-1")
		o.awsRegion = "us-east-1"
	}
	if o.awsProfile == "" {
		fmt.Println("using default AWS profile: osd-staging-2")
		o.awsProfile = "osd-staging-2"
	}
	return nil
}

func writeFile(user string, content string) error {
	fileName := fmt.Sprintf("aws-iam-%s", user)

	file, err := os.CreateTemp("", fileName)
	if err != nil {
		return fmt.Errorf("failed to create credential file %s\n%s", file.Name(), err)
	}
	defer file.Close()

	if err = file.Chmod(0600); err != nil {
		return fmt.Errorf("failed to set permissions on file %s\n%s", file.Name(), err)
	}

	if _, err = file.WriteString(content); err != nil {
		return fmt.Errorf("failed to write AccessKey to file %s\n%s", file.Name(), err)
	}

	fmt.Printf("AWS credentials file created in %s\n", file.Name())
	return nil
}

func (o *iamOptions) run() error {
	// Build the base AWS client using the provide credentials (profile or env vars)
	awsClient, err := awsprovider.NewAwsClient(o.awsProfile, o.awsRegion, "")
	if err != nil {
		return fmt.Errorf("could not build AWS Client: %s", err)
	}

	// Get the right partition for the final ARN
	partition, err := awsprovider.GetAwsPartition(awsClient)
	if err != nil {
		return fmt.Errorf("could not get AWS partition: %s", err)
	}

	// Generate a session name using the SRE's kerberos ID
	sessionName, err := osdCloud.GenerateRoleSessionName(awsClient)
	if err != nil {
		return fmt.Errorf("could not generate Session Name: %s", err)
	}

	// Use OrganizationAccountAccessRole
	assumedRoleCreds, err := osdCloud.GenerateOrganizationAccountAccessCredentials(awsClient, o.awsAccountID, sessionName, partition)
	if err != nil {
		return fmt.Errorf("could not build AWS Client for OrganizationAccountAccessRole: %s", err)
	}

	// Variable with credential to be used by the impersonate aws client
	impersonateAwsCredentials := awsprovider.ClientInput{
		AccessKeyID:     *assumedRoleCreds.AccessKeyId,
		SecretAccessKey: *assumedRoleCreds.SecretAccessKey,
		SessionToken:    *assumedRoleCreds.SessionToken,
		Region:          o.awsRegion,
	}

	// Create a new impersonated AWS client
	impersonateAwsClient, err := awsprovider.NewAwsClientWithInput(&impersonateAwsCredentials)
	if err != nil {
		return fmt.Errorf("could create impersonated AWS Client: %s", err)
	}

	// Check if IAM user already exists
	iamUserExist, err := awsprovider.CheckIAMUserExists(impersonateAwsClient, &o.kerberosUser)
	if !iamUserExist {
		// Create IAM user
		err := awsprovider.CreateIAMUserAndAttachPolicy(impersonateAwsClient, &o.kerberosUser, &arnPolicy)
		if err != nil {
			return fmt.Errorf("error Creating the IAM user: %s", err)
		}

		// Create AccessKey
		newAccessKey, err := impersonateAwsClient.CreateAccessKey(&iam.CreateAccessKeyInput{UserName: &o.kerberosUser})
		if err != nil {
			return fmt.Errorf("error creating the use AccessKey: %s", err)
		}

		// Creates the file with aws IAM user credentials
		content := fmt.Sprintf("[%v-dev-account]\naws_access_key_id = %v\naws_secret_access_key = %v\n", *newAccessKey.AccessKey.UserName, *newAccessKey.AccessKey.AccessKeyId, *newAccessKey.AccessKey.SecretAccessKey)
		if err := writeFile(*newAccessKey.AccessKey.UserName, content); err != nil {
			return err
		}

		return nil

	} else {
		if o.rotate {
			// Get existing AccessKeys
			existingKeys, err := impersonateAwsClient.ListAccessKeys(&iam.ListAccessKeysInput{UserName: &o.kerberosUser})
			if err != nil {
				return fmt.Errorf("error getting existing AccessKeys: %s", err)
			}

			// Delete existing AccessKeys
			for i := range existingKeys.AccessKeyMetadata {
				_, err := impersonateAwsClient.DeleteAccessKey(&iam.DeleteAccessKeyInput{AccessKeyId: existingKeys.AccessKeyMetadata[i].AccessKeyId, UserName: &o.kerberosUser})
				if err != nil {
					return fmt.Errorf("error deleting existing AccessKeys: %s", err)
				}
			}

			// Create new AccessKey
			newAccessKey, err := impersonateAwsClient.CreateAccessKey(&iam.CreateAccessKeyInput{UserName: &o.kerberosUser})
			if err != nil {
				return fmt.Errorf("error creating AccessKey: %s", err)
			}

			// Creates the file with aws IAM user credentials
			content := fmt.Sprintf("[%v-dev-account]\naws_access_key_id = %v\naws_secret_access_key = %v\n", *newAccessKey.AccessKey.UserName, *newAccessKey.AccessKey.AccessKeyId, *newAccessKey.AccessKey.SecretAccessKey)
			if err := writeFile(*newAccessKey.AccessKey.UserName, content); err != nil {
				return err
			}

			return nil
		}

		// Print message if user exists and rotate not defined
		fmt.Printf("IAM User %s already exists in AWS account %s\n", o.kerberosUser, o.awsAccountID)
		return err
	}

}
