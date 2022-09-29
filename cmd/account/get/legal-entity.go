package get

import (
	"context"
	"fmt"

	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	outputflag "github.com/openshift/osdctl/cmd/getoutput"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/cmd/common"
)

// newCmdGetLegalEntity implements the get legal-entity command which get
// the legal entity information related to the specified AWS Account ID
func newCmdGetLegalEntity(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newGetLegalEntityOptions(streams, flags, client, globalOpts)
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
	kubeCli       client.Client
	GlobalOptions *globalflags.GlobalOptions
}

type legalEntityResponse struct {
	Name string `json:"name" yaml:"name"`
	Id   string `json:"id" yaml:"id"`
}

func (f legalEntityResponse) String() string {

	return fmt.Sprintf("  Name: %s\n  Id: %s\n", f.Name, f.Id)

}

func newGetLegalEntityOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *getLegalEntityOptions {
	return &getLegalEntityOptions{
		flags:         flags,
		IOStreams:     streams,
		kubeCli:       client,
		GlobalOptions: globalOpts,
	}
}

func (o *getLegalEntityOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.accountID == "" {
		return cmdutil.UsageErrorf(cmd, accountIDRequired)
	}

	o.output = o.GlobalOptions.Output
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

			err := outputflag.PrintResponse(o.output, resp)
			if err != nil {
				fmt.Println("Error while printing response: ", err.Error())
				return err
			}
		}
	}

	// matched account not found
	_, err := fmt.Fprintf(o.IOStreams.Out, "Account matched for AWS Account ID %s not found\n", o.accountID)
	if err != nil {
		return err
	}

	return nil
}
