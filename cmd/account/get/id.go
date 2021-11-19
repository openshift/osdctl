package get

import (
	"context"
	"fmt"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/k8s"
)

// newCmdGetAWSAccount implements the reset command which resets the specified account cr
func newCmdGetAWSAccount(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newGetAWSAccountOptions(streams, flags, client)
	getAWSAccountCmd := &cobra.Command{
		Use:               "aws-account",
		Short:             "Get AWS Account ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	getAWSAccountCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	getAWSAccountCmd.Flags().StringVarP(&ops.accountName, "account", "a", "", "Account CR Name")
	getAWSAccountCmd.Flags().StringVarP(&ops.accountClaimName, "account-claim", "c", "", "Account Claim CR Name")
	getAWSAccountCmd.Flags().StringVarP(&ops.accountClaimNamespace, "account-claim-ns", "n", "", "Account Claim CR Namespace")

	return getAWSAccountCmd
}

// getAWSAccountOptions defines the struct for running get aws-account command
type getAWSAccountOptions struct {
	accountNamespace      string
	accountName           string
	accountClaimName      string
	accountClaimNamespace string

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newGetAWSAccountOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *getAWSAccountOptions {
	return &getAWSAccountOptions{
		flags:     flags,
		IOStreams: streams,
		kubeCli:   client,
	}
}

func (o *getAWSAccountOptions) complete(cmd *cobra.Command, _ []string) error {
	// account claim CR name and account CR name cannot be empty at the same time
	if o.accountName == "" && o.accountClaimName == "" {
		return cmdutil.UsageErrorf(cmd, "Account CR Name and AccountClaim CR Name cannot be empty at the same time")
	}

	if o.accountName != "" && o.accountClaimName != "" {
		return cmdutil.UsageErrorf(cmd, "Account CR Name and AccountClaim CR Name cannot be set at the same time")
	}

	return nil
}

func (o *getAWSAccountOptions) run() error {
	ctx := context.TODO()

	var accountID string
	if o.accountName != "" {
		account, err := k8s.GetAWSAccount(ctx, o.kubeCli, o.accountNamespace, o.accountName)
		if err != nil {
			return err
		}
		accountID = account.Spec.AwsAccountID
	} else {
		var (
			accountClaim awsv1alpha1.AccountClaim
			account      awsv1alpha1.Account
		)
		if err := o.kubeCli.Get(ctx, types.NamespacedName{
			Namespace: o.accountClaimNamespace,
			Name:      o.accountClaimName,
		}, &accountClaim); err != nil {
			return err
		}

		if accountClaim.Spec.AccountLink == "" {
			fmt.Fprintf(o.IOStreams.Out, "Account matched for AccountClaim %s not found\n", o.accountClaimName)
			return nil
		}

		if err := o.kubeCli.Get(ctx, types.NamespacedName{
			Namespace: o.accountNamespace,
			Name:      accountClaim.Spec.AccountLink,
		}, &account); err != nil {
			return err
		}

		accountID = account.Spec.AwsAccountID
	}

	fmt.Fprintln(o.IOStreams.Out, accountID)
	return nil
}
