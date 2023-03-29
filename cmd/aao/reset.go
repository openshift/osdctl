package aao

import (
	"context"
	"fmt"

	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func newCmdReset(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newResetOptions(streams, flags, client)
	resetCmd := &cobra.Command{
		Use:               "reset <legalEntityID>",
		Short:             "Reset the unnused AWS Accounts for a specific Legal Entity ID",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	resetCmd.Flags().StringVar(&ops.legalEntityID, "legalEntityID", "", "The legal entity ID you'd like to reset")
	resetCmd.Flags().IntVar(&ops.count, "count", 10, "How many accounts you'd like to reset in this legal entity. Defaults to 10")

	return resetCmd
}

// resetOptions defines the struct for running reset command
type resetOptions struct {
	legalEntityID string
	count         int

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newResetOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *resetOptions {
	return &resetOptions{
		flags:     flags,
		IOStreams: streams,
		kubeCli:   client,
	}
}

func (o *resetOptions) complete(cmd *cobra.Command, args []string) error {
	if o.legalEntityID == "" {
		return cmdutil.UsageErrorf(cmd, "Legal Entity ID argument is required")
	}
	if o.count <= 0 {
		return cmdutil.UsageErrorf(cmd, "Count must be greater than 0")
	}
	return nil
}

func (o *resetOptions) run() error {
	ctx := context.TODO()
	var accounts awsv1alpha1.AccountList
	if err := o.kubeCli.List(ctx, &accounts, &client.ListOptions{
		Namespace: "aws-account-operator",
	}); err != nil {
		return err
	}

	resetCount := 0
	for _, account := range accounts.Items {
		if resetCount < o.count {
			if !account.Status.Claimed && account.Spec.LegalEntity.ID == o.legalEntityID && !account.Spec.BYOC {
				fmt.Printf("%d. Resetting account %s\n", resetCount, account.Name)
				// TODO refactor osdctl account reset to be called here
				resetCount++
			}
		}
	}
	return nil
}
