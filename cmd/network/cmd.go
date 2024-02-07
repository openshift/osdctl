package network

import (
	"fmt"

	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdNetwork implements the base cluster deployment command
func NewCmdNetwork(streams genericclioptions.IOStreams, client *k8s.LazyClient) *cobra.Command {
	netCmd := &cobra.Command{
		Use:               "network",
		Short:             "network related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	netCmd.AddCommand(newCmdPacketCapture(streams, client))
	netCmd.AddCommand(NewCmdValidateEgress())
	return netCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help(): ", err.Error())
		return
	}
}
