package cmd

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sts"
	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osd-utils-cli/pkg/k8s"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
)

const (
	defaultRegion = "us-east-1"
)

// newCmdConsole implements the Console command which Consoles the specified account cr
func newCmdConsole(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newConsoleOptions(streams, flags)
	consoleCmd := &cobra.Command{
		Use:                   "console [flags] [options]",
		Short:                 "generate a AWS console URL on the fly",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
	}

	consoleCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", awsAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	consoleCmd.Flags().StringVarP(&ops.accountName, "account-name", "a", "", "The AWS account cr we need to create AWS console URL for")
	consoleCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "The AWS account ID we need to create AWS console URL for")
	consoleCmd.Flags().StringVarP(&ops.region, "aws-region", "r", defaultRegion, "specify AWS region")
	consoleCmd.Flags().Int64VarP(&ops.consoleDuration, "duration", "d", 3600, "The duration of the console session. "+
		"Default value is 3600 seconds(1 hour)")

	return consoleCmd
}

// consoleOptions defines the struct for running Console command
type consoleOptions struct {
	accountName      string
	accountID        string
	accountNamespace string
	consoleDuration  int64

	// AWS config
	region string

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newConsoleOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *consoleOptions {
	return &consoleOptions{
		flags:     flags,
		IOStreams: streams,
	}
}

func (o *consoleOptions) complete(cmd *cobra.Command) error {
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

func (o *consoleOptions) run() error {
	var err error
	awsClient, err := awsprovider.NewAwsClient(o.region)
	if err != nil {
		return err
	}

	ctx := context.TODO()
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

	callerIdentityOutput, err := awsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}

	roleName := awsv1alpha1.AccountOperatorIAMRole
	consoleURL, err := awsprovider.RequestSignInToken(awsClient, &o.consoleDuration,
		callerIdentityOutput.UserId, aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)))
	if err != nil {
		return err
	}
	fmt.Fprintf(o.IOStreams.Out, "The AWS Console URL is:\n%s\n", consoleURL)

	return nil
}
