package get

import (
	"context"
	"fmt"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/printer"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// newCmdGetAccountClaim implements the get account-claim command which get
// the Account Claim CR related to the specified AWS Account ID
func newCmdGetAccountClaim(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newGetAccountClaimOptions(streams, flags, client, globalOpts)
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
	kubeCli       client.Client
	GlobalOptions *globalflags.GlobalOptions
}

func newGetAccountClaimOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *getAccountClaimOptions {
	return &getAccountClaimOptions{
		flags:         flags,
		printFlags:    printer.NewPrintFlags(),
		IOStreams:     streams,
		kubeCli:       client,
		GlobalOptions: globalOpts,
	}
}

func (o *getAccountClaimOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.accountID == "" && o.accountName == "" {
		return cmdutil.UsageErrorf(cmd, "AWS account ID and Account CR Name cannot be empty at the same time")
	}
	if o.accountID != "" && o.accountName != "" {
		return cmdutil.UsageErrorf(cmd, "AWS account ID and Account CR Name cannot be set at the same time")
	}

	o.output = o.GlobalOptions.Output
	return nil
}

func (o *getAccountClaimOptions) run() error {
	ctx := context.TODO()

	var (
		accountClaims []awsv1alpha1.AccountClaim
		err           error
	)

	if o.accountName != "" {
		var accountClaim awsv1alpha1.AccountClaim
		accountClaim, err = getAccountClaim(ctx, o.kubeCli, o.accountName)
		accountClaims = []awsv1alpha1.AccountClaim{accountClaim}
	} else {
		accountClaims, err = GetAccountClaimFromAccountID(ctx, o.kubeCli, o.accountID, o.accountNamespace)
	}

	if err != nil {
		return err
	}

	if o.output == "" {
		p := printer.NewTablePrinter(o.IOStreams.Out, 20, 1, 3, ' ')
		p.AddRow([]string{"Namespace", "Name"})
		for _, accountClaim := range accountClaims {
			p.AddRow([]string{accountClaim.Namespace, accountClaim.Name})
		}
		return p.Flush()
	}

	resourcePrinter, err := o.printFlags.ToPrinter(o.output)
	if err != nil {
		return err
	}

	for _, accountClaim := range accountClaims {
		err = resourcePrinter.PrintObj(&accountClaim, o.Out)
		if err != nil {
			return nil
		}
	}

	return nil
}

func GetAccountClaimFromAccountID(ctx context.Context, kubeCli client.Client, accountID string, namespace string) (accountClaims []awsv1alpha1.AccountClaim, err error) {
	accounts := awsv1alpha1.AccountList{}
	err = kubeCli.List(ctx, &accounts, &client.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return
	}

	errors := []error{}
	for _, a := range accounts.Items {
		if a.Spec.AwsAccountID == accountID {
			claim, claimerr := getAccountClaim(ctx, kubeCli, a.Name)
			if claimerr != nil {
				errors = append(errors, claimerr)
				claim.Name = "ClaimNotFound"
				claim.Namespace = "ClaimNotFound"
			}
			accountClaims = append(accountClaims, claim)
		}
	}
	if len(accountClaims) == 0 {
		err = fmt.Errorf("account matched for AWS Account ID %s not found", accountID)
		return
	}
	if len(errors) > 0 {
		err = fmt.Errorf("%v", errors)
		return
	}
	return

}

func getAccountClaim(ctx context.Context, kubeCli client.Client, accountName string) (accountClaim awsv1alpha1.AccountClaim, err error) {
	accountClaims := awsv1alpha1.AccountClaimList{}
	if err = kubeCli.List(ctx, &accountClaims); err != nil {
		return
	}

	for _, a := range accountClaims.Items {
		if a.Spec.AccountLink == accountName {
			accountClaim = a
			return
		}
	}
	err = fmt.Errorf("AccountClaim matched for Account CR %s not found\n", accountName)
	return
}
