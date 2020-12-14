package network

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdNetwork implements the base cluster deployment command
func NewCmdNetwork(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	netCmd := &cobra.Command{
		Use:               "network",
		Short:             "network related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	netCmd.AddCommand(newCmdPacketCapture(streams, flags))
	return netCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
