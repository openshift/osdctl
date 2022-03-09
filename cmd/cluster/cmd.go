package cluster

import (
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdClusterHealth implements the base cluster health command
func NewCmdCluster(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:               "cluster",
		Short:             "Provides information for a specified cluster",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	clusterCmd.AddCommand(newCmdHealth(streams, flags, globalOpts))
	clusterCmd.AddCommand(newCmdloggingCheck(streams, flags, globalOpts))
	return clusterCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
