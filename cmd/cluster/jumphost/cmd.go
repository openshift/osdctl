package jumphost

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdJumphost implements the cluster utility
func NewCmdJumphost(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, kubeClient client.Client) *cobra.Command {
	jumphostCmd := &cobra.Command{
		Use:               "jumphost",
		Short:             "Jumphost Creation/Deletion for a given cluster",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	jumphostCmd.AddCommand(newCmdGenerateKey(streams, flags))
	jumphostCmd.AddCommand(newCmdInit(streams, flags))
	return jumphostCmd
}
