package account

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/printers"
	"time"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osd-utils-cli/cmd/common"
	"github.com/openshift/osd-utils-cli/pkg/k8s"
	"github.com/openshift/osd-utils-cli/pkg/printer"
)

// newCmdList implements the list command to list account crs
func newCmdList(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newListOptions(streams, flags)
	listCmd := &cobra.Command{
		Use:               "list",
		Short:             "List AWS Account CR",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	ops.printFlags.AddFlags(listCmd)
	listCmd.Flags().StringVarP(&ops.output, "output", "o", "", "Output format. One of: json|yaml|jsonpath=...|jsonpath-file=... see jsonpath template [http://kubernetes.io/docs/user-guide/jsonpath].")
	listCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	listCmd.Flags().StringVarP(&ops.reused, "reuse", "r", "",
		"Filter account CRs by reused or not. Supported values are true, false. Otherwise it lists all accounts")
	listCmd.Flags().StringVarP(&ops.claimed, "claim", "c", "",
		"Filter account CRs by claimed or not. Supported values are true, false. Otherwise it lists all accounts")
	listCmd.Flags().StringVar(&ops.state, "state", "all", "Account cr state. The default value is all to display all the crs")

	return listCmd
}

// listOptions defines the struct for running list command
type listOptions struct {
	accountNamespace string

	reused  string
	claimed string
	state   string

	output string

	flags      *genericclioptions.ConfigFlags
	printFlags *printer.PrintFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newListOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *listOptions {
	return &listOptions{
		flags:      flags,
		printFlags: printer.NewPrintFlags(),
		IOStreams:  streams,
	}
}

func (o *listOptions) complete(cmd *cobra.Command, _ []string) error {
	switch o.state {
	// display all the crs
	case "all":

	// valid value, continue
	case "Creating", "Pending", "PendingVerification",
		"Failed", "Ready", "":

	// throw error
	default:
		return cmdutil.UsageErrorf(cmd, "unsupported account state "+o.state)
	}

	switch o.reused {
	case "", "true", "false":
	default:
		return cmdutil.UsageErrorf(cmd, "unsupported reused status filter "+o.reused)
	}

	switch o.claimed {
	case "", "true", "false":
	default:
		return cmdutil.UsageErrorf(cmd, "unsupported claimed status filter "+o.claimed)
	}

	var err error
	o.kubeCli, err = k8s.NewClient(o.flags)
	if err != nil {
		return err
	}

	return nil
}

func (o *listOptions) run() error {
	ctx := context.TODO()

	var (
		accounts        awsv1alpha1.AccountList
		outputAccounts  awsv1alpha1.AccountList
		resourcePrinter printers.ResourcePrinter
		matched         bool
		reused          bool
		claimed         bool
		err             error
	)
	if o.reused != "" {
		if o.reused == "true" {
			reused = true
		}
	}

	if o.claimed != "" {
		if o.claimed == "true" {
			claimed = true
		}
	}

	if err := o.kubeCli.List(ctx, &accounts, &client.ListOptions{
		Namespace: o.accountNamespace}); err != nil {
		return err
	}

	if o.output != "" {
		outputAccounts = awsv1alpha1.AccountList{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "List",
			},
			Items: make([]awsv1alpha1.Account, 0),
		}

		resourcePrinter, err = o.printFlags.ToPrinter(o.output)
		if err != nil {
			return err
		}
	}

	p := printer.NewTablePrinter(o.IOStreams.Out, 20, 1, 3, ' ')
	p.AddRow([]string{"Name", "State", "AWS ACCOUNT ID", "Last Probe Time", "Last Transition Time", "Message"})

	for _, account := range accounts.Items {
		if o.claimed != "" {
			if account.Status.Claimed != claimed {
				continue
			}
		}
		if o.reused != "" {
			if account.Status.Reused != reused {
				continue
			}
		}

		if o.state != "all" && account.Status.State != o.state {
			continue
		}

		if o.output != "" {
			outputAccounts.Items = append(outputAccounts.Items, account)
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

		rows := []string{
			account.Name,
			account.Status.State,
			account.Spec.AwsAccountID,
			lastProbeTime.String(),
			lastTransitionTime.String(),
			message,
		}

		p.AddRow(rows)

		// this is used to mark whether there are matched accounts or not
		matched = true
	}

	if o.output != "" {
		return resourcePrinter.PrintObj(&outputAccounts, o.Out)
	}

	if matched {
		return p.Flush()
	}
	return nil
}
