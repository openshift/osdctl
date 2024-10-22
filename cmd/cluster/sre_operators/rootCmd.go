package sre_operators

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewCmdSREOperators(streams genericclioptions.IOStreams, client client.Client) *cobra.Command {
	sreOperatorsCmd := &cobra.Command{
		Use:               "sre-operators",
		Short:             "SRE operator related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	sreOperatorsCmd.AddCommand(newCmdList(streams, client))
	sreOperatorsCmd.AddCommand(newCmdDescribe(streams, client))

	return sreOperatorsCmd
}
