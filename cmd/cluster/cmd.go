package cluster

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdClusterHealth implements the base cluster health command
func NewCmdCluster(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:               "cluster",
		Short:             "Provides vitals of an AWS cluster",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	clusterCmd.AddCommand(newCmdHealth(streams, flags))
	return clusterCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
