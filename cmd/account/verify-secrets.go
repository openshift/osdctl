package account

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/service/sts"
	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/k8s"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
)

const (
	verifySecretsUsage = "The verify-secrets command should have only 0 or 1 arguments"
)

// newCmdVerifySecrets implements the verify-secrets command
// which verifies AWS credentials managed by AWS Account Operator
func newCmdVerifySecrets(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newVerifySecretsOptions(streams, flags, client)
	verifySecretsCmd := &cobra.Command{
		Use:               "verify-secrets [<account name>]",
		Short:             "Verify AWS Account CR IAM User credentials",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
		Aliases: []string{"verify-secret"},
	}

	verifySecretsCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	verifySecretsCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")
	verifySecretsCmd.Flags().BoolVarP(&ops.all, "all", "A", false, "Verify all Account CRs")

	return verifySecretsCmd
}

// verifySecretsOptions defines the struct for running verify command
type verifySecretsOptions struct {
	accountName      string
	accountNamespace string

	verbose bool
	all     bool

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newVerifySecretsOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *verifySecretsOptions {
	return &verifySecretsOptions{
		flags:     flags,
		IOStreams: streams,
		kubeCli:   client,
	}
}

func (o *verifySecretsOptions) complete(cmd *cobra.Command, args []string) error {
	if len(args) > 1 {
		return cmdutil.UsageErrorf(cmd, verifySecretsUsage)
	}

	if len(args) == 1 {
		o.accountName = args[0]
	}

	return nil
}

func (o *verifySecretsOptions) run() error {
	ctx := context.TODO()
	var (
		awsClient   awsprovider.Client
		credentials []*awsSecret
		err         error
		allErr      bool
	)
	if o.all {
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
		if o.accountName == "" {
			return fmt.Errorf("Please provide an account CR name")
		}
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

		// Add osdCcsAdmin credentials to be validated for CCS accounts
		if account.Spec.BYOC {
			creds, err := k8s.GetAWSAccountCredentials(ctx, o.kubeCli, account.Spec.ClaimLinkNamespace, "byoc")
			if err != nil {
				return err
			}
			credentials = append(credentials, &awsSecret{
				secret: "byoc",
				awsCreds: &awsprovider.AwsClientInput{
					AccessKeyID:     creds.AccessKeyID,
					SecretAccessKey: creds.SecretAccessKey,
				},
			})
		}
	}

	for _, cred := range credentials {
		if o.verbose {
			fmt.Fprintln(o.IOStreams.Out, "Start validating secret "+cred.secret)
		}
		awsClient, err = awsprovider.NewAwsClientWithInput(cred.awsCreds)
		if err != nil {
			fmt.Fprintf(o.IOStreams.Out, "Failed to create AWS client with secret %s\n", cred.secret)
			if o.all {
				allErr = true
				continue
			}
			return err
		}
		if _, err := awsClient.GetCallerIdentity(
			&sts.GetCallerIdentityInput{}); err != nil {
			fmt.Fprintf(o.IOStreams.Out, "Failed to get caller identity with secret %s\n", cred.secret)
			if o.all {
				allErr = true
				continue
			}
			return err
		}
	}

	if allErr {
		fmt.Fprintf(o.IOStreams.Out, "Some credentials are invalid\n")
		return errors.New("AccountCredentialError")
	}

	if !o.all {
		fmt.Fprintf(o.IOStreams.Out, "Credentials valid for %s\n", o.accountName)
	} else {
		fmt.Fprintf(o.IOStreams.Out, "Credentials valid for all Account CRs\n")
	}

	return nil
}

type awsSecret struct {
	awsCreds *awsprovider.AwsClientInput
	secret   string
}
