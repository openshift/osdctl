package get

import (
	"context"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v2"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/printer"
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

func GetOutput(cmd *cobra.Command) string {

	out, err := cmd.Flags().GetString("output")
	if err != nil {
		panic(err)
	}

	return out
}

// getLegalEntityOptions defines the struct for running get legal-entity command
type getLegalEntityOptions struct {
	accountID        string
	accountNamespace string
	output           string

	flags      *genericclioptions.ConfigFlags
	printFlags *printer.PrintFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

type legalEntityResponse struct {
	Name string `json:"name",yaml:"name"`
	Id   string `json:"id",yaml:""id`
}

func (f legalEntityResponse) String() string {

	return fmt.Sprintf("  Name: %s\n  Id: %s\n", f.Name, f.Id)

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

	o.output = GetOutput(cmd)
	if o.output != "" && o.output != "json" && o.output != "yaml" {
		return cmdutil.UsageErrorf(cmd, "Invalid output value")
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

			if o.output == "json" {

				accountsToJson, err := json.MarshalIndent(resp, "", "    ")
				if err != nil {
					return err
				}

				fmt.Println(string(accountsToJson))

			} else if o.output == "yaml" {

				accountIdToYaml, err := yaml.Marshal(resp)
				if err != nil {
					return err
				}

				fmt.Println(string(accountIdToYaml))

			} else {
				fmt.Fprintln(o.IOStreams.Out, resp)
			}
		}
	}

	// matched account not found
	fmt.Fprintf(o.IOStreams.Out, "Account matched for AWS Account ID %s not found\n", o.accountID)

	return nil
}
