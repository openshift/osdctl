package get

import (
	"context"
	"fmt"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/printer"
)

// newCmdGetAccountClaim implements the get account-claim command which get
// the Account Claim CR related to the specified AWS Account ID
func newCmdGetAccountClaim(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newGetAccountClaimOptions(streams, flags, client)
	getAccountClaimCmd := &cobra.Command{
		Use:               "account-claim",
		Short:             "Get AWS Account Claim CR",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	ops.printFlags.AddFlags(getAccountClaimCmd)
	getAccountClaimCmd.Flags().StringVarP(&ops.output, "output", "o", "", "Output format. One of: json|yaml|jsonpath=...|jsonpath-file=... see jsonpath template [http://kubernetes.io/docs/user-guide/jsonpath].")
	getAccountClaimCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	getAccountClaimCmd.Flags().StringVarP(&ops.accountName, "account", "a", "", "Account CR Name")
	getAccountClaimCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "AWS account ID")

	return getAccountClaimCmd
}

// getAccountClaimOptions defines the struct for running get account-claim command
type getAccountClaimOptions struct {
	accountName      string
	accountID        string
	accountNamespace string

	output string

	flags      *genericclioptions.ConfigFlags
	printFlags *printer.PrintFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newGetAccountClaimOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *getAccountClaimOptions {
	return &getAccountClaimOptions{
		flags:      flags,
		printFlags: printer.NewPrintFlags(),
		IOStreams:  streams,
		kubeCli:    client,
	}
}

func (o *getAccountClaimOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.accountID == "" && o.accountName == "" {
		return cmdutil.UsageErrorf(cmd, "AWS account ID and Account CR Name cannot be empty at the same time")
	}
	if o.accountID != "" && o.accountName != "" {
		return cmdutil.UsageErrorf(cmd, "AWS account ID and Account CR Name cannot be set at the same time")
	}

	return nil
}

func (o *getAccountClaimOptions) run() error {
	ctx := context.TODO()

	var (
		accounts         awsv1alpha1.AccountList
		accountClaims    awsv1alpha1.AccountClaimList
		accountCRName    string
		accountClaimName string
		accountClaim     awsv1alpha1.AccountClaim
	)

	if o.accountName != "" {
		accountCRName = o.accountName
	} else {
		if err := o.kubeCli.List(ctx, &accounts, &client.ListOptions{
			Namespace: o.accountNamespace,
		}); err != nil {
			return err
		}

		for _, a := range accounts.Items {
			if a.Spec.AwsAccountID == o.accountID {
				accountCRName = a.Name
				break
			}
		}
		if accountCRName == "" {
			return fmt.Errorf("Account matched for AWS Account ID %s not found\n", o.accountID)
		}
	}

	if err := o.kubeCli.List(ctx, &accountClaims); err != nil {
		return nil
	}

	for _, a := range accountClaims.Items {
		if a.Spec.AccountLink == accountCRName {
			accountClaimName = a.Name
			accountClaim = a
			break
		}
	}
	if accountClaimName == "" {
		return fmt.Errorf("AccountClaim matched for Account CR %s not found\n", accountCRName)
	}

	if o.output == "" {
		p := printer.NewTablePrinter(o.IOStreams.Out, 20, 1, 3, ' ')
		p.AddRow([]string{"Namespace", "Name"})
		p.AddRow([]string{accountClaim.Namespace, accountClaimName})
		return p.Flush()
	}

	resourcePrinter, err := o.printFlags.ToPrinter(o.output)
	if err != nil {
		return err
	}

	return resourcePrinter.PrintObj(&accountClaim, o.Out)
}
