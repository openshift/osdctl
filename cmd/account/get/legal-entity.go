package get

import (
	"context"
	"fmt"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	outputflag "github.com/openshift/osdctl/cmd/getoutput"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/k8s"
)

// newCmdGetLegalEntity implements the get legal-entity command which get
// the legal entity information related to the specified AWS Account ID
func newCmdGetLegalEntity(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newGetLegalEntityOptions(streams, flags, client)
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
	output           string

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

type legalEntityResponse struct {
	Name string `json:"name" yaml:"name"`
	Id   string `json:"id" yaml:"id"`
}

func (f legalEntityResponse) String() string {

	return fmt.Sprintf("  Name: %s\n  Id: %s\n", f.Name, f.Id)

}

func newGetLegalEntityOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *getLegalEntityOptions {
	return &getLegalEntityOptions{
		flags:     flags,
		IOStreams: streams,
		kubeCli:   client,
	}
}

func (o *getLegalEntityOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.accountID == "" {
		return cmdutil.UsageErrorf(cmd, accountIDRequired)
	}

	output, err := outputflag.GetOutput(cmd)
	if err != nil {
		return err
	}
	o.output = output

	o.kubeCli, err = k8s.NewClient(o.flags)
	if err != nil {
		return err
	}

	return nil
}

func (o *getLegalEntityOptions) run() error {
	ctx := context.TODO()

	var (
		accounts awsv1alpha1.AccountList
	)

	if err := o.kubeCli.List(ctx, &accounts, &client.ListOptions{
		Namespace: o.accountNamespace,
	}); err != nil {
		return err
	}

	for _, account := range accounts.Items {
		if account.Spec.AwsAccountID == o.accountID {

			resp := legalEntityResponse{
				Name: account.Spec.LegalEntity.Name,
				Id:   account.Spec.LegalEntity.ID,
			}

			outputflag.PrintResponse(o.output, resp)
		}
	}

	// matched account not found
	fmt.Fprintf(o.IOStreams.Out, "Account matched for AWS Account ID %s not found\n", o.accountID)

	return nil
}
