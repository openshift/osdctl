package hypershift

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func NewCmdHypershift(streams genericclioptions.IOStreams) *cobra.Command {
	hypershiftCmd := &cobra.Command{
		Use:               "hypershift",
		Short:             "Hypershift utilities for SRE workflows",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	hypershiftCmd.AddCommand(NewCmdInfo(streams))

	return hypershiftCmd
}
