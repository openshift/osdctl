package sts

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdClusterHealth implements the base cluster health command
func NewCmdSts(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:               "sts",
		Short:             "STS related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	clusterCmd.AddCommand(newCmdPolicyDiff(streams, flags, client))
	clusterCmd.AddCommand(newCmdPolicy(streams, flags, client))
	return clusterCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
