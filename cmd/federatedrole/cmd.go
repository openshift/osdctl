package federatedrole

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdFederatedRole implements the basic federated role command
func NewCmdFederatedRole(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	getCmd := &cobra.Command{
		Use:               "federatedrole",
		Short:             "federated role related commands",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	getCmd.AddCommand(newCmdApply(streams, flags, client))

	return getCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
