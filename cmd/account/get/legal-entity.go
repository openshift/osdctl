package get

import (
	"context"
	"fmt"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osd-utils-cli/cmd/common"
	"github.com/openshift/osd-utils-cli/pkg/k8s"
	"github.com/openshift/osd-utils-cli/pkg/printer"
)

// newCmdGetLegalEntity implements the get legal-entity command which get
// the legal entity information related to the specified AWS Account ID
func newCmdGetLegalEntity(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newGetLegalEntityOptions(streams, flags)
	getLegalEntityCmd := &cobra.Command{
		Use:               "legal-entity",
		Short:             "Get AWS Account Legal Entity",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	getLegalEntityCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	getLegalEntityCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "AWS account ID")

	return getLegalEntityCmd
}

// getLegalEntityOptions defines the struct for running get legal-entity command
type getLegalEntityOptions struct {
	accountID        string
	accountNamespace string

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newGetLegalEntityOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *getLegalEntityOptions {
	return &getLegalEntityOptions{
		flags:     flags,
		IOStreams: streams,
	}
}

func (o *getLegalEntityOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.accountID == "" {
		return cmdutil.UsageErrorf(cmd, accountIDRequired)
	}

	var err error
	o.kubeCli, err = k8s.NewClient(o.flags)
	if err != nil {
		return err
	}

	return nil
}

func (o *getLegalEntityOptions) run() error {
	ctx := context.TODO()

	var accounts awsv1alpha1.AccountList
	if err := o.kubeCli.List(ctx, &accounts, &client.ListOptions{
		Namespace: o.accountNamespace,
	}); err != nil {
		return err
	}

	for _, account := range accounts.Items {
		if account.Spec.AwsAccountID == o.accountID {
			p := printer.NewTablePrinter(o.IOStreams.Out, 20, 1, 3, ' ')
			p.AddRow([]string{"LegalEntity Name", "LegalEntity ID"})
			p.AddRow([]string{account.Spec.LegalEntity.Name, account.Spec.LegalEntity.ID})
			return p.Flush()
		}
	}

	// matched account not found
	fmt.Fprintf(o.IOStreams.Out, "Account matched for AWS Account ID %s not found\n", o.accountID)

	return nil
}
