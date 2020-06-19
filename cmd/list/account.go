package list

import (
	"context"
	"time"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osd-utils-cli/pkg/k8s"
	"github.com/openshift/osd-utils-cli/pkg/printer"
)

// newCmdListAccounts implements the list account command to list account crs
func newCmdListAccount(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newListAccountOptions(streams, flags)
	listAccountCmd := &cobra.Command{
		Use:                   "account [flags] [options]",
		Short:                 "List AWS Account CR",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	listAccountCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", awsAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	listAccountCmd.Flags().BoolVarP(&ops.reused, "reuse", "r", false, "Only list reused accounts CR if true")
	listAccountCmd.Flags().StringVar(&ops.state, "state", "", "Account cr state. If not specified, it will list all crs by default.")

	return listAccountCmd
}

// listAccountOptions defines the struct for running list account command
type listAccountOptions struct {
	accountNamespace string

	reused bool
	state  string

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newListAccountOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *listAccountOptions {
	return &listAccountOptions{
		flags:     flags,
		IOStreams: streams,
	}
}

func (o *listAccountOptions) complete(cmd *cobra.Command, _ []string) error {
	switch o.state {
	// state doesn't set, continue
	case "":

	// valid value, continue
	case "Creating", "Pending", "PendingVerification",
		"Failed", "Ready":

	// throw error
	default:
		return cmdutil.UsageErrorf(cmd, "unsupported account state "+o.state)
	}

	var err error
	o.kubeCli, err = k8s.NewClient(o.flags)
	if err != nil {
		return err
	}

	return nil
}

func (o *listAccountOptions) run() error {
	ctx := context.TODO()
	var accounts awsv1alpha1.AccountList
	if err := o.kubeCli.List(ctx, &accounts, &client.ListOptions{
		Namespace: o.accountNamespace}); err != nil {
		return err
	}

	var matched bool
	p := printer.NewTablePrinter(o.IOStreams.Out, 20, 1, 3, ' ')
	p.AddRow([]string{"Name", "State", "AWS ACCOUNT ID", "Last Probe Time", "Last Transition Time", "Message"})
	for _, account := range accounts.Items {
		if o.reused != account.Status.Reused {
			continue
		}

		if o.state != "" && account.Status.State != o.state {
			continue
		}

		conditionLen := len(account.Status.Conditions)
		var (
			lastProbeTime      time.Time
			lastTransitionTime time.Time
			message            string
		)
		if conditionLen > 0 {
			lastProbeTime = account.Status.Conditions[conditionLen-1].LastProbeTime.Time
			lastTransitionTime = account.Status.Conditions[conditionLen-1].LastTransitionTime.Time
			message = account.Status.Conditions[conditionLen-1].Message
		}
		p.AddRow([]string{
			account.Name,
			account.Status.State,
			account.Spec.AwsAccountID,
			lastProbeTime.String(),
			lastTransitionTime.String(),
			message,
		})

		// this is used to mark whether there are matched accounts or not
		matched = true
	}

	if matched {
		return p.Flush()
	}
	return nil
}
