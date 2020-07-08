package list

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdGet implements the get command to get AWS Account related resources
func NewCmdList(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	listCmd := &cobra.Command{
		Use:               "list",
		Short:             "List resources",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	listCmd.AddCommand(newCmdListAccount(streams, flags))
	listCmd.AddCommand(newCmdListAccountClaim(streams, flags))

	return listCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
