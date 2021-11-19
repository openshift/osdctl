package list

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdGet implements the get command to get AWS Account related resources
func NewCmdList(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	listCmd := &cobra.Command{
		Use:               "list",
		Short:             "List resources",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	listCmd.AddCommand(newCmdListAccount(streams, flags, client))
	listCmd.AddCommand(newCmdListAccountClaim(streams, flags, client))

	return listCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
