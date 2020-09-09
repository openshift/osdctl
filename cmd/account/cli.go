package account

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sts"
	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osd-utils-cli/cmd/common"
	"github.com/openshift/osd-utils-cli/pkg/k8s"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
)

// newCmdCli implements the Cli command which generates temporary STS cli credentials for the specified account cr
func newCmdCli(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newCliOptions(streams, flags)
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

	cliCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	cliCmd.Flags().StringVarP(&ops.accountName, "account-name", "a", "", "The AWS Account CR name to generate the credentials for")
	cliCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "The AWS Account ID we need to create temporary AWS credentials for")
	cliCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "The Internal Cluster ID from Hive to create AWS console URL for")
	cliCmd.Flags().StringVarP(&ops.profile, "aws-profile", "p", "", "specify AWS profile")
	cliCmd.Flags().StringVarP(&ops.cfgFile, "aws-config", "c", "", "specify AWS config file path")
	cliCmd.Flags().StringVarP(&ops.region, "aws-region", "r", common.DefaultRegion, "specify AWS region")
	cliCmd.Flags().Int64VarP(&ops.cliDuration, "duration", "d", 3600, "The duration of the cli token. "+
		"Default value is 3600 seconds(1 hour)")
	cliCmd.Flags().BoolVarP(&ops.verbose, "verbose", "v", false, "Verbose output")

	return cliCmd
}

// cliOptions defines the struct for running the cli command
type cliOptions struct {
	accountName      string
	accountID        string
	accountNamespace string
	clusterID        string
	cliDuration      int64

	// AWS config
	region  string
	profile string
	cfgFile string

	verbose bool

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newCliOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cliOptions {
	return &cliOptions{
		flags:     flags,
		IOStreams: streams,
	}
}

func (o *cliOptions) complete(cmd *cobra.Command) error {
	// account CR name and account ID cannot be empty at the same time
	if o.accountName == "" && o.accountID == "" && o.clusterID == "" {
		return cmdutil.UsageErrorf(cmd, "AWS account CR name, AWS account ID and Cluster ID cannot be empty at the same time")
	}

	if !o.hasOnlyOneTarget() {
		return cmdutil.UsageErrorf(cmd, "AWS account CR name, AWS account ID, or Cluster ID cannot be set at the same time")
	}

	// only initialize kubernetes client when account name or cluster ID is set
	if o.accountName != "" || o.clusterID != "" {
		var err error
		o.kubeCli, err = k8s.NewClient(o.flags)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *cliOptions) hasOnlyOneTarget() bool {
	targets := []string{o.accountName, o.accountID, o.clusterID}
	targetCount := 0
	for _, t := range targets {
		if t != "" {
			targetCount++
		}
	}
	return targetCount == 1
}

func (o *cliOptions) run() error {
	var err error
	awsClient, err := awsprovider.NewAwsClient(o.profile, o.region, o.cfgFile)
	if err != nil {
		return err
	}

	ctx := context.TODO()
	var accountID string
	if o.clusterID != "" {
		accountClaim, err := k8s.GetAccountClaimFromClusterID(ctx, o.kubeCli, o.clusterID)
		if err != nil {
			return err
		}
		if accountClaim == nil {
			return fmt.Errorf("Could not find any accountClaims for cluster with ID: %s", o.clusterID)
		}
		if accountClaim.Spec.AccountLink == "" {
			return fmt.Errorf("An unexpected error occured: the AccountClaim has no Account")
		}
		o.accountName = accountClaim.Spec.AccountLink
	}
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
		klog.Error("Fail to get caller identity. Could you please validate the credentials?")
		return err
	}
	if o.verbose {
		fmt.Fprintln(o.Out, callerIdentityOutput)
	}

	roleName := awsv1alpha1.AccountOperatorIAMRole
	credentials, err := awsprovider.GetAssumeRoleCredentials(awsClient, &o.cliDuration,
		callerIdentityOutput.UserId, aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)))
	if err != nil {
		return err
	}
	fmt.Fprintf(o.IOStreams.Out, "Temporary AWS Credentials:\n%s\n", credentials)

	return nil
}
