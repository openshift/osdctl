package clusterdeployment

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdClusterDeployment implements the base cluster deployment command
func NewCmdClusterDeployment(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	cdCmd := &cobra.Command{
		Use:               "clusterdeployment",
		Short:             "cluster deployment related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	cdCmd.AddCommand(newCmdList(streams, flags))
	return cdCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
