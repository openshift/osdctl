package account

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/sts"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/k8s"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// newCmdGenerateSecret implements the generate-secret command which generates an new set of IAM User credentials
func newCmdGenerateSecret(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newGenerateSecretOptions(streams, flags, client)
	generateSecretCmd := &cobra.Command{
		Use:               "generate-secret <IAM User name>",
		Short:             "Generate IAM credentials secret",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
		Aliases: []string{"generate-secrets"},
	}

	generateSecretCmd.Flags().StringVarP(&ops.accountName, "account-name", "a", "", "AWS Account CR name")
	generateSecretCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	generateSecretCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "AWS Account ID")
	generateSecretCmd.Flags().StringVarP(&ops.profile, "aws-profile", "p", "", "specify AWS profile")
	generateSecretCmd.Flags().StringVar(&ops.secretName, "secret-name", "", "Specify name of the generated secret")
	generateSecretCmd.Flags().StringVar(&ops.secretNamespace, "secret-namespace", "aws-account-operator", "Specify namespace of the generated secret")
	generateSecretCmd.Flags().BoolVar(&ops.quiet, "quiet", false, "Suppress logged output")
	generateSecretCmd.Flags().BoolVar(&ops.ccs, "ccs", false, "Only generate specific secret for osdCcsAdmin. Requires Account CR name")

	return generateSecretCmd
}

// generateSecretOptions defines the struct for running generate-secret command
type generateSecretOptions struct {
	accountName      string
	accountID        string
	accountNamespace string
	iamUsername      string

	secretName      string
	secretNamespace string
	quiet           bool
	ccs             bool
	outputPath      string

	// AWS config
	region  string
	profile string
	cfgFile string

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newGenerateSecretOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *generateSecretOptions {
	return &generateSecretOptions{
		flags:     flags,
		IOStreams: streams,
		kubeCli:   client,
	}
}

func (o *generateSecretOptions) complete(cmd *cobra.Command, args []string) error {

	// Opinionated config for CCS rotation
	if o.ccs {
		// If no account CR name is provided via the -a option, try and get it via argument
		if o.accountName == "" {
			return cmdutil.UsageErrorf(cmd, "Account CR name argument is required")
		}

		return nil
	}

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

	return nil
}

func (o *generateSecretOptions) run() error {
	if o.ccs {
		return o.generateCcsSecret()
	}

	ctx := context.TODO()
	var err error
	awsSetupClient, err := awsprovider.NewAwsClient(o.profile, o.region, o.cfgFile)
	if err != nil {
		return err
	}

	// Get the accountID
	var accountID string
	if o.accountName != "" {
		account, err := k8s.GetAWSAccount(ctx, o.kubeCli, o.accountNamespace, o.accountName)
		if err != nil {
			return err
		}
		if account.Spec.AwsAccountID != "" {
			accountID = account.Spec.AwsAccountID
		} else {
			return fmt.Errorf("account CR is missing AWS Account ID")
		}
	} else {
		accountID = o.accountID
	}

	// Ensure creds are valid
	callerIdentityOutput, err := awsSetupClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}

	arn, err := arn.Parse(aws.StringValue(callerIdentityOutput.Arn))
	if err != nil {
		return err
	}

	// Assume
	roleArn := aws.String(fmt.Sprintf("arn:%s:iam::%s:role/%s", arn.Partition, accountID, awsv1alpha1.AccountOperatorIAMRole))
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

	username := aws.String(o.iamUsername)
	ok, err := awsprovider.CheckIAMUserExists(awsClient, username)
	if err != nil {
		return err
	}

	// if the specified user does not exist, create one
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
	if !o.quiet {
		fmt.Fprintln(o.IOStreams.Out, secret)
	}

	if o.outputPath != "" {
		outputPath, err := filepath.Abs(o.outputPath)
		if err != nil {
			return err
		}
		// set permission to 0600 to ensure, only owner has access
		return ioutil.WriteFile(outputPath, []byte(secret), 0600)
	}

	return nil
}

func (o *generateSecretOptions) generateCcsSecret() error {

	ctx := context.TODO()
	var err error
	awsSetupClient, err := awsprovider.NewAwsClient(o.profile, o.region, o.cfgFile)
	if err != nil {
		return err
	}

	account, err := k8s.GetAWSAccount(ctx, o.kubeCli, common.AWSAccountNamespace, o.accountName)
	if err != nil {
		return err
	}

	// Ensure AWS calls are successful with client
	callerIdentityOutput, err := awsSetupClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}

	accountIDSuffixLabel, ok := account.Labels["iamUserId"]
	if !ok {
		return fmt.Errorf("No label on Account CR for IAM User")
	}

	// Get the aws-account-operator configmap
	cm := &corev1.ConfigMap{}
	cmErr := o.kubeCli.Get(context.TODO(), types.NamespacedName{Namespace: common.AWSAccountNamespace, Name: common.DefaultConfigMap}, cm)
	if cmErr != nil {
		return fmt.Errorf("There was an error getting the ConfigMap to get the SRE Access Role %s", cmErr)
	}
	// Get the ARN value
	SREAccessARN := cm.Data["CCS-Access-Arn"]
	if SREAccessARN == "" {
		return fmt.Errorf("SRE Access ARN is missing from configmap")
	}

	// Assume the ARN
	srepRoleCredentials, err := awsprovider.GetAssumeRoleCredentials(awsSetupClient, aws.Int64(900), callerIdentityOutput.UserId, &SREAccessARN)
	if err != nil {
		return err
	}

	// Create client with the SREP role
	srepRoleClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.AwsClientInput{
		AccessKeyID:     *srepRoleCredentials.AccessKeyId,
		SecretAccessKey: *srepRoleCredentials.SecretAccessKey,
		SessionToken:    *srepRoleCredentials.SessionToken,
		Region:          "us-east-1",
	})
	if err != nil {
		return err
	}

	// Role chain to assume ManagedOpenShift-Support-{uid}
	roleArn := aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", account.Spec.AwsAccountID, "ManagedOpenShift-Support-"+accountIDSuffixLabel))
	credentials, err := awsprovider.GetAssumeRoleCredentials(srepRoleClient, aws.Int64(900),
		callerIdentityOutput.UserId, roleArn)
	if err != nil {
		return err
	}

	// Create client with the chain assumed role
	awsAssumedRoleClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.AwsClientInput{
		AccessKeyID:     *credentials.AccessKeyId,
		SecretAccessKey: *credentials.SecretAccessKey,
		SessionToken:    *credentials.SessionToken,
		Region:          "us-east-1",
	})

	// Create new set of Access Keys for osdCcsAdmin
	newKey, err := awsAssumedRoleClient.CreateAccessKey(&iam.CreateAccessKeyInput{
		UserName: aws.String(common.OSDCcsAdminIAM),
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

	if !o.quiet {
		fmt.Fprintln(o.IOStreams.Out, secret)
	}

	return nil
}
