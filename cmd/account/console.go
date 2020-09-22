package account

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sts"
	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osd-utils-cli/cmd/common"
	"github.com/openshift/osd-utils-cli/pkg/k8s"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
)

// newCmdConsole implements the Console command which Consoles the specified account cr
func newCmdConsole(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newConsoleOptions(streams, flags)
	consoleCmd := &cobra.Command{
		Use:               "console",
		Short:             "Generate an AWS console URL on the fly",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
	}

	consoleCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	consoleCmd.Flags().StringVarP(&ops.accountName, "account-name", "a", "", "The AWS account cr we need to create AWS console URL for")
	consoleCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "The AWS account ID we need to create AWS console URL for -- This argument will not work for CCS accounts")
	consoleCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "The Internal Cluster ID from Hive to create AWS console URL for")
	consoleCmd.Flags().StringVarP(&ops.profile, "aws-profile", "p", "", "specify AWS profile")
	consoleCmd.Flags().StringVarP(&ops.cfgFile, "aws-config", "c", "", "specify AWS config file path")
	consoleCmd.Flags().StringVarP(&ops.region, "aws-region", "r", common.DefaultRegion, "specify AWS region")
	consoleCmd.Flags().Int64VarP(&ops.consoleDuration, "duration", "d", 3600, "The duration of the console session. "+
		"Default value is 3600 seconds(1 hour)")
	consoleCmd.Flags().BoolVarP(&ops.verbose, "verbose", "v", false, "Verbose output")

	return consoleCmd
}

// consoleOptions defines the struct for running Console command
type consoleOptions struct {
	accountName      string
	accountID        string
	accountNamespace string
	clusterID        string
	consoleDuration  int64

	// AWS config
	region  string
	profile string
	cfgFile string

	verbose bool

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
	if o.accountName == "" && o.accountID == "" && o.clusterID == "" {
		return cmdutil.UsageErrorf(cmd, "AWS account CR name, AWS account ID and Cluster ID cannot be empty at the same time")
	}

	if !o.hasOnlyOneTarget() {
		return cmdutil.UsageErrorf(cmd, "AWS account CR name, AWS account ID and Cluster ID cannot be set at the same time")
	}

	// only initialize kubernetes client when account name is set or cluster ID is set
	if o.accountName != "" || o.clusterID != "" {
		var err error
		o.kubeCli, err = k8s.NewClient(o.flags)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *consoleOptions) hasOnlyOneTarget() bool {
	targets := []string{o.accountName, o.accountID, o.clusterID}
	targetCount := 0
	for _, t := range targets {
		if t != "" {
			targetCount++
		}
	}
	return targetCount == 1
}

func (o *consoleOptions) run() error {
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
	var isBYOC bool
	var acctSuffix string
	if o.accountName != "" {
		account, err := k8s.GetAWSAccount(ctx, o.kubeCli, o.accountNamespace, o.accountName)
		if err != nil {
			return err
		}
		accountID = account.Spec.AwsAccountID
		isBYOC = account.Spec.BYOC
		acctSuffix = account.Labels["iamUserId"]
	} else {
		accountID = o.accountID
		isBYOC = false
	}

	callerIdentityOutput, err := awsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		klog.Error("Fail to get caller identity. Could you please validate the credentials?")
		return err
	}
	if o.verbose {
		fmt.Fprintln(o.Out, callerIdentityOutput)
	}
	splitArn := strings.Split(*callerIdentityOutput.Arn, "/")
	username := splitArn[1]
	sessionName := fmt.Sprintf("RH-SRE-%s", username)

	// If BYOC we need to role-chain to use the right creds.
	// Use the OrgAccess Role by default, override if BYOC
	roleName := awsv1alpha1.AccountOperatorIAMRole

	// TODO: Come back to this and do a lookup for the account CR if the account ID is the only one set so we can do this too.
	if isBYOC {
		cm := &corev1.ConfigMap{}
		err = o.kubeCli.Get(ctx, types.NamespacedName{Namespace: awsv1alpha1.AccountCrNamespace, Name: awsv1alpha1.DefaultConfigMap}, cm)
		if err != nil {
			klog.Error("There was an error getting the configmap.")
			return err
		}
		roleArn := cm.Data["CCS-Access-Arn"]

		if roleArn == "" {
			klog.Error("Empty SRE Jump Role in ConfigMap")
			return fmt.Errorf("Empty ConfigMap Value")
		}

		// Build the role-name for Access:
		if acctSuffix == "" {
			klog.Error("Unexpected error parsing the account CR suffix")
			return fmt.Errorf("Unexpected error parsing the account CR suffix.")
		}
		roleName = fmt.Sprintf("BYOCAdminAccess-%s", acctSuffix)

		// Get STS Credentials
		if o.verbose {
			fmt.Printf("Elevating Access to SRE Jump Role for user %s\n", sessionName)
		}
		creds, err := awsprovider.GetAssumeRoleCredentials(awsClient, &o.consoleDuration, aws.String(sessionName), aws.String(roleArn))
		if err != nil {
			klog.Error("Failed to get jump-role creds for CCS")
			return err
		}

		awsClientInput := &awsprovider.AwsClientInput{
			AccessKeyID:     *creds.AccessKeyId,
			SecretAccessKey: *creds.SecretAccessKey,
			SessionToken:    *creds.SessionToken,
			Region:          "us-east-1",
		}
		// New Client with STS Credentials
		awsClient, err = awsprovider.NewAwsClientWithInput(awsClientInput)
		if err != nil {
			klog.Error("Failed to assume jump-role for CCS")
			return err
		}
	}

	consoleURL, err := awsprovider.RequestSignInToken(awsClient, &o.consoleDuration,
		aws.String(sessionName), aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)))
	if err != nil {
		return err
	}
	fmt.Fprintf(o.IOStreams.Out, "The AWS Console URL is:\n%s\n", consoleURL)

	return nil
}
