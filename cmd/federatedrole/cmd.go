package federatedrole

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdFederatedRole implements the basic federated role command
func NewCmdFederatedRole(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	getCmd := &cobra.Command{
		Use:               "federatedrole",
		Short:             "federated role related commands",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	getCmd.AddCommand(newCmdApply(streams, flags))

	return getCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
