package list

import (
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdGet implements the get command to get AWS Account related resources
func NewCmdList(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	listCmd := &cobra.Command{
		Use:               "list",
		Short:             "List resources",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	listCmd.AddCommand(newCmdListAccount(streams, flags, client, globalOpts))
	listCmd.AddCommand(newCmdListAccountClaim(streams, flags, client, globalOpts))

	return listCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
