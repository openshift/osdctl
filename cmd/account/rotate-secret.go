package account

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/sts"
	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osd-utils-cli/cmd/common"
	"github.com/openshift/osd-utils-cli/pkg/k8s"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
)

// newCmdRotateSecret implements the rotate-secret command which rotate IAM User credentials
func newCmdRotateSecret(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newRotateSecretOptions(streams, flags)
	rotateSecretCmd := &cobra.Command{
		Use:               "rotate-secret <IAM User name>",
		Short:             "Rotate IAM credentials secret",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	rotateSecretCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")

	rotateSecretCmd.Flags().StringVarP(&ops.accountName, "account-name", "a", "", "AWS Account CR name")
	rotateSecretCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "AWS Account ID")
	rotateSecretCmd.Flags().StringVarP(&ops.profile, "aws-profile", "p", "", "specify AWS profile")
	rotateSecretCmd.Flags().StringVarP(&ops.cfgFile, "aws-config", "c", "", "specify AWS config file path")
	rotateSecretCmd.Flags().StringVarP(&ops.region, "aws-region", "r", common.DefaultRegion, "Specify AWS region")
	rotateSecretCmd.Flags().StringVar(&ops.secretName, "secret-name", "byoc", "Specify name of the generated secret")
	rotateSecretCmd.Flags().StringVar(&ops.secretNamespace, "secret-namespace", "aws-account-operator", "Specify namespace of the generated secret")
	rotateSecretCmd.Flags().BoolVar(&ops.printSecret, "print", true, "Print the generated secret")
	rotateSecretCmd.Flags().StringVarP(&ops.outputPath, "output", "o", "", "Output path for secret yaml file")

	return rotateSecretCmd
}

// rotateSecretOptions defines the struct for running rotate-iam command
type rotateSecretOptions struct {
	accountName      string
	accountID        string
	accountNamespace string
	iamUsername      string

	secretName      string
	secretNamespace string
	printSecret     bool
	outputPath      string

	// AWS config
	region  string
	profile string
	cfgFile string

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newRotateSecretOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *rotateSecretOptions {
	return &rotateSecretOptions{
		flags:     flags,
		IOStreams: streams,
	}
}

func (o *rotateSecretOptions) complete(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "IAM User name argument is required")
	}
	o.iamUsername = args[0]

	// account CR name and account ID cannot be empty at the same time
	if o.accountName == "" && o.accountID == "" {
		return cmdutil.UsageErrorf(cmd, "AWS account CR name and AWS account ID cannot be empty at the same time")
	}

	if o.accountName != "" && o.accountID != "" {
		return cmdutil.UsageErrorf(cmd, "AWS account CR name and AWS account ID cannot be set at the same time")
	}

	// only initialize kubernetes client when account name is set
	if o.accountName != "" {
		var err error
		o.kubeCli, err = k8s.NewClient(o.flags)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *rotateSecretOptions) run() error {
	ctx := context.TODO()
	var err error
	awsSetupClient, err := awsprovider.NewAwsClient(o.profile, o.region, o.cfgFile)
	if err != nil {
		return err
	}

	var accountID string
	if o.accountName != "" {
		account, err := k8s.GetAWSAccount(ctx, o.kubeCli, o.accountNamespace, o.accountName)
		if err != nil {
			return err
		}
		accountID = account.Spec.AwsAccountID
	} else {
		accountID = o.accountID
	}

	callerIdentityOutput, err := awsSetupClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}

	roleArn := aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, awsv1alpha1.AccountOperatorIAMRole))
	credentials, err := awsprovider.GetAssumeRoleCredentials(awsSetupClient, aws.Int64(900),
		callerIdentityOutput.UserId, roleArn)
	if err != nil {
		return err
	}

	awsClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.AwsClientInput{
		AccessKeyID:     *credentials.AccessKeyId,
		SecretAccessKey: *credentials.SecretAccessKey,
		SessionToken:    *credentials.SessionToken,
		Region:          o.region,
	})
	if err != nil {
		return err
	}

	// if the account ID is specified, will
	if o.accountID != "" {
		callerIdentityOutput, err = awsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
		if err != nil {
			return err
		}
		if *callerIdentityOutput.Account != o.accountID {
			return fmt.Errorf("account ID %s does not match the input account ID %s",
				*callerIdentityOutput.Account, o.accountID)
		}
	}

	username := aws.String(o.iamUsername)
	ok, err := awsprovider.CheckIAMUserExists(awsClient, username)
	if err != nil {
		return err
	}

	// the specified user not exist, create one
	if !ok {
		policyArn := aws.String("arn:aws:iam::aws:policy/AdministratorAccess")
		if err := awsprovider.CreateIAMUserAndAttachPolicy(awsClient,
			username, policyArn); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(o.IOStreams.Out, "User %s exists, deleting existing access keys now.\n", o.iamUsername)
		if err := awsprovider.DeleteUserAccessKeys(awsClient, username); err != nil {
			return err
		}
	}

	newKey, err := awsClient.CreateAccessKey(&iam.CreateAccessKeyInput{
		UserName: username,
	})
	if err != nil {
		return err
	}

	secret := k8s.NewAWSSecret(
		o.secretName,
		o.secretNamespace,
		*newKey.AccessKey.AccessKeyId,
		*newKey.AccessKey.SecretAccessKey,
	)
	if o.printSecret {
		fmt.Fprintln(o.IOStreams.Out, secret)
	}

	if o.outputPath != "" {
		outputPath, err := filepath.Abs(o.outputPath)
		if err != nil {
			return err
		}
		return ioutil.WriteFile(outputPath, []byte(secret), 0644)
	}

	return nil
}
