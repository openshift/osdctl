package list

import (
	"context"

	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/printer"
)

// newCmdListAccount implements the list account command to list account claim crs
func newCmdListAccountClaim(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newListAccountClaimOptions(streams, flags, client, globalOpts)
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
	kubeCli       client.Client
	GlobalOptions *globalflags.GlobalOptions
}

func newListAccountClaimOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *listAccountClaimOptions {
	return &listAccountClaimOptions{
		flags:         flags,
		IOStreams:     streams,
		kubeCli:       client,
		GlobalOptions: globalOpts,
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

	o.output = o.GlobalOptions.Output
	return nil
}

func (o *listAccountClaimOptions) run() error {
	ctx := context.TODO()
	var claims awsv1alpha1.AccountClaimList
	if err := o.kubeCli.List(ctx, &claims, &client.ListOptions{}); err != nil {
		return err
	}

	var matched bool
	p := printer.NewTablePrinter(o.IOStreams.Out, 20, 1, 3, ' ')
	p.AddRow([]string{"Namespace", "Name", "State", "Account", "AWS OU"})
	for i, claim := range claims.Items {
		if o.state != "" && string(claim.Status.State) != o.state {
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
		if matched {
			if o.output == "" {
				return p.Flush()
			}
			resourcePrinter, err := o.printFlags.ToPrinter(o.output)
			if err != nil {
				return err
			}

			return resourcePrinter.PrintObj(&claims.Items[i], o.Out)
		}
	}
	return nil
}
