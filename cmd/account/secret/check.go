package secret

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/sts"
	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osd-utils-cli/cmd/common"
	"github.com/openshift/osd-utils-cli/pkg/k8s"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
)

const (
	checkSecretsUsage = "The check-secrets command should have only 0 or 1 arguments"
)

// newCmdCheckSecrets implements the check-secrets command
// which checks AWS credentials managed by AWS Account Operator
func newCmdCheckSecrets(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newCheckSecretsOptions(streams, flags)
	checkSecretsCmd := &cobra.Command{
		Use:               "check [<account name>]",
		Short:             "Check AWS Account CR IAM User credentials",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	checkSecretsCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	checkSecretsCmd.Flags().BoolVarP(&ops.verbose, "verbose", "v", false, "Verbose output")

	return checkSecretsCmd
}

// checkSecretsOptions defines the struct for running check command
type checkSecretsOptions struct {
	accountName      string
	accountNamespace string

	verbose bool

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newCheckSecretsOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *checkSecretsOptions {
	return &checkSecretsOptions{
		flags:     flags,
		IOStreams: streams,
	}
}

func (o *checkSecretsOptions) complete(cmd *cobra.Command, args []string) error {
	if len(args) > 1 {
		return cmdutil.UsageErrorf(cmd, checkSecretsUsage)
	}

	if len(args) == 1 {
		o.accountName = args[0]
	}

	var err error
	o.kubeCli, err = k8s.NewClient(o.flags)
	if err != nil {
		return err
	}

	return nil
}

func (o *checkSecretsOptions) run() error {
	ctx := context.TODO()
	var (
		awsClient   awsprovider.Client
		credentials []*awsSecret
		err         error
	)
	if o.accountName == "" {
		var accounts awsv1alpha1.AccountList
		if err := o.kubeCli.List(ctx, &accounts, &client.ListOptions{
			Namespace: o.accountNamespace,
		}); err != nil {
			return err
		}

		for _, account := range accounts.Items {
			if account.Spec.IAMUserSecret == "" {
				continue
			}
			if o.verbose {
				fmt.Fprintln(o.IOStreams.Out, "Getting AWS Credentials for account "+account.Name)
			}
			creds, err := k8s.GetAWSAccountCredentials(ctx, o.kubeCli, o.accountNamespace, account.Spec.IAMUserSecret)
			if err != nil {
				if apierrors.IsNotFound(err) && account.Status.State != "Creating" {
					fmt.Fprintf(o.IOStreams.Out, "Account %s doesn't have associate credentials, state %s",
						account.Name, account.Status.State)
				}
				continue
			}
			credentials = append(credentials, &awsSecret{
				secret: account.Spec.IAMUserSecret,
				awsCreds: &awsprovider.AwsClientInput{
					AccessKeyID:     creds.AccessKeyID,
					SecretAccessKey: creds.SecretAccessKey,
				},
			})
		}
	} else {
		account, err := k8s.GetAWSAccount(ctx, o.kubeCli, o.accountNamespace, o.accountName)
		if err != nil {
			return err
		}
		if account.Spec.IAMUserSecret == "" {
			return fmt.Errorf("account %s doesn't have associate credentials", account.Name)
		}
		if o.verbose {
			fmt.Fprintln(o.IOStreams.Out, "Getting AWS Credentials for account "+account.Name)
		}
		creds, err := k8s.GetAWSAccountCredentials(ctx, o.kubeCli, o.accountNamespace, account.Spec.IAMUserSecret)
		if err != nil {
			return err
		}
		credentials = append(credentials, &awsSecret{
			secret: account.Spec.IAMUserSecret,
			awsCreds: &awsprovider.AwsClientInput{
				AccessKeyID:     creds.AccessKeyID,
				SecretAccessKey: creds.SecretAccessKey,
			},
		})
	}

	for _, cred := range credentials {
		if o.verbose {
			fmt.Fprintln(o.IOStreams.Out, "Start validating secret "+cred.secret)
		}
		awsClient, err = awsprovider.NewAwsClientWithInput(cred.awsCreds)
		if err != nil {
			fmt.Fprintf(o.IOStreams.Out, "Failed to create AWS client with secret %s\n", cred.secret)
			continue
		}
		if _, err := awsClient.GetCallerIdentity(
			&sts.GetCallerIdentityInput{}); err != nil {
			fmt.Fprintf(o.IOStreams.Out, "Failed to get caller identity with secret %s\n", cred.secret)
			continue
		}
	}

	return nil
}

type awsSecret struct {
	awsCreds *awsprovider.AwsClientInput
	secret   string
}
