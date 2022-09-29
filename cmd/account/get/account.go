package get

import (
	"context"
	"fmt"

	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/printer"
)

// newCmdGetAccount implements the get account command which get the Account CR
// related to the specified AWS Account ID or the specified Account Claim CR
func newCmdGetAccount(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newGetAccountOptions(streams, flags, client, globalOpts)
	getAccountCmd := &cobra.Command{
		Use:               "account",
		Short:             "Get AWS Account CR",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	ops.printFlags.AddFlags(getAccountCmd)
	getAccountCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	getAccountCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "AWS account ID")
	getAccountCmd.Flags().StringVarP(&ops.accountClaimName, "account-claim", "c", "", "Account Claim CR name")
	getAccountCmd.Flags().StringVarP(&ops.accountClaimNamespace, "account-claim-ns", "n", "", "Account Claim CR namespace")

	return getAccountCmd
}

// getAccountOptions defines the struct for running get account command
type getAccountOptions struct {
	accountID             string
	accountNamespace      string
	accountClaimName      string
	accountClaimNamespace string

	output string

	flags      *genericclioptions.ConfigFlags
	printFlags *printer.PrintFlags
	genericclioptions.IOStreams
	kubeCli       client.Client
	GlobalOptions *globalflags.GlobalOptions
}

func newGetAccountOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *getAccountOptions {
	return &getAccountOptions{
		flags:         flags,
		printFlags:    printer.NewPrintFlags(),
		IOStreams:     streams,
		kubeCli:       client,
		GlobalOptions: globalOpts,
	}
}

func (o *getAccountOptions) complete(cmd *cobra.Command, _ []string) error {
	// account claim CR name and account ID cannot be empty at the same time
	if o.accountID == "" && o.accountClaimName == "" {
		return cmdutil.UsageErrorf(cmd, "AWS account ID and AccountClaim CR Name cannot be empty at the same time")
	}

	if o.accountID != "" && o.accountClaimName != "" {
		return cmdutil.UsageErrorf(cmd, "AWS account ID and AccountClaim CR Name cannot be set at the same time")
	}

	o.output = o.GlobalOptions.Output
	return nil
}

func (o *getAccountOptions) run() error {
	ctx := context.TODO()

	var (
		accountCRName string
		account       *awsv1alpha1.Account
	)
	if o.accountClaimName != "" {
		accountClaim, err := k8s.GetAWSAccountClaim(
			ctx, o.kubeCli,
			o.accountClaimNamespace,
			o.accountClaimName,
		)
		if err != nil {
			return err
		}
		// there is no related account
		if accountClaim.Spec.AccountLink == "" {
			fmt.Fprintf(o.IOStreams.Out, "Account matched for AccountClaim %s not found\n", o.accountClaimName)
			return nil
		}

		accountCRName = accountClaim.Spec.AccountLink
		account, err = k8s.GetAWSAccount(ctx, o.kubeCli, o.accountNamespace, accountCRName)
		if err != nil {
			return err
		}
	} else {
		var accounts awsv1alpha1.AccountList
		if err := o.kubeCli.List(ctx, &accounts, &client.ListOptions{
			Namespace: o.accountNamespace,
		}); err != nil {
			return err
		}

		for i, a := range accounts.Items {
			if a.Spec.AwsAccountID == o.accountID {
				accountCRName = a.Name
				account = &accounts.Items[i]
				break
			}
		}
		if accountCRName == "" {
			fmt.Fprintf(o.IOStreams.Out, "Account matched for AWS Account ID %s not found\n", o.accountID)
			return nil
		}
	}

	if o.output == "" {
		fmt.Fprintln(o.IOStreams.Out, accountCRName)
		return nil
	}

	resourcePrinter, err := o.printFlags.ToPrinter(o.output)
	if err != nil {
		return err
	}

	return resourcePrinter.PrintObj(account, o.Out)
}
