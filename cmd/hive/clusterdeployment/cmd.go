package clusterdeployment

import (
	"fmt"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdClusterDeployment implements the base cluster deployment command
func NewCmdClusterDeployment(streams genericclioptions.IOStreams, client client.Client) *cobra.Command {
	cdCmd := &cobra.Command{
		Use:               "clusterdeployment",
		Short:             "cluster deployment related utilities",
		Aliases:           []string{"cd"},
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	cdCmd.AddCommand(newCmdList(streams, client))
	cdCmd.AddCommand(newCmdListResources(streams, client))
	return cdCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help()", err.Error())
	}
}
