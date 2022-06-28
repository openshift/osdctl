package network

import (
	"fmt"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdNetwork implements the base cluster deployment command
func NewCmdNetwork(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	netCmd := &cobra.Command{
		Use:               "network",
		Short:             "network related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	netCmd.AddCommand(newCmdPacketCapture(streams, flags, client))
	return netCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help(): ", err.Error())
		return
	}
}
