package ci

import "github.com/spf13/cobra"

func NewCmdCI() *cobra.Command {
	ciCmd := &cobra.Command{
		Use:               "ci",
		Short:             "Commands for managing CI pipelines and jobs",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}
	ciCmd.AddCommand(newCmdRetriggerPipeline())
	return ciCmd
}
