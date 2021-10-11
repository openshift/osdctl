package list

import (
	"context"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/printers"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/printer"
)

func GetOutput(cmd *cobra.Command) string {

	out, err := cmd.Flags().GetString("output")
	if err != nil {
		panic(err)
	}

	return out
}

// newCmdListAccount implements the list account command to list account claim crs
func newCmdListAccountClaim(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newListAccountClaimOptions(streams, flags)
	listAccountClaimCmd := &cobra.Command{
		Use:               "account-claim",
		Short:             "List AWS Account Claim CR",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	listAccountClaimCmd.Flags().StringVar(&ops.state, "state", "", "Account cr state. If not specified, it will list all crs by default.")

	return listAccountClaimCmd
}

// listAccountOptions defines the struct for running list account command
type listAccountClaimOptions struct {
	state  string
	output string

	flags      *genericclioptions.ConfigFlags
	printFlags *printer.PrintFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newListAccountClaimOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *listAccountClaimOptions {
	return &listAccountClaimOptions{
		flags:     flags,
		IOStreams: streams,
	}
}

func (o *listAccountClaimOptions) complete(cmd *cobra.Command, _ []string) error {
	switch o.state {
	// state doesn't set, continue
	case "":

	// valid value, continue
	case "Error", "Pending", "Ready":

	// throw error
	default:
		return cmdutil.UsageErrorf(cmd, "unsupported account claim state "+o.state)
	}

	o.output = GetOutput(cmd)

	var err error
	o.kubeCli, err = k8s.NewClient(o.flags)
	if err != nil {
		return err
	}

	return nil
}

func (o *listAccountClaimOptions) run() error {
	ctx := context.TODO()
	var (
		outputClaims    awsv1alpha1.AccountClaimList
		claims          awsv1alpha1.AccountClaimList
		resourcePrinter printers.ResourcePrinter
		matched         bool
		err             error
	)
	if err := o.kubeCli.List(ctx, &claims, &client.ListOptions{}); err != nil {
		return err
	}

	if o.output != "" {
		outputClaims = awsv1alpha1.AccountClaimList{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "List",
			},
			Items: make([]awsv1alpha1.AccountClaim, 0),
		}

		resourcePrinter, err = o.printFlags.ToPrinter(o.output)
		if err != nil {
			return err
		}
	}

	p := printer.NewTablePrinter(o.IOStreams.Out, 20, 1, 3, ' ')
	p.AddRow([]string{"Namespace", "Name", "State", "Account", "AWS OU"})
	for _, claim := range claims.Items {

		if o.state != "" && string(claim.Status.State) != o.state {
			continue
		}

		if o.output != "" {
			outputClaims.Items = append(outputClaims.Items, claim)
			continue
		}

		p.AddRow([]string{
			claim.Namespace,
			claim.Name,
			string(claim.Status.State),
			claim.Spec.AccountLink,
			claim.Spec.AccountOU,
		})

		// this is used to mark whether there are matched accounts or not
		matched = true
	}

	if o.output != "" {
		return resourcePrinter.PrintObj(&outputClaims, o.Out)
	}

	if matched {
		return p.Flush()
	}

	return nil
}
