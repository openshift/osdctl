package list

import (
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdGet implements the get command to get AWS Account related resources
func NewCmdList(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	listCmd := &cobra.Command{
		Use:               "list",
		Short:             "List resources",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	listCmd.AddCommand(newCmdListAccount(streams, flags, globalOpts))
	listCmd.AddCommand(newCmdListAccountClaim(streams, flags, globalOpts))

	return listCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
