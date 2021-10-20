package sts

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdClusterHealth implements the base cluster health command
func NewCmdSts(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:               "sts",
		Short:             "STS related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	clusterCmd.AddCommand(newCmdPolicyDiff(streams, flags))
	clusterCmd.AddCommand(newCmdPolicy(streams, flags))
	return clusterCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
