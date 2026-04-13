package rhobs

import (
	"github.com/spf13/cobra"
)

func NewCmdRhobs() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "rhobs",
		Short:             "RHOBS.next related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	cmd.AddCommand(newCmdCell())
	cmd.AddCommand(newCmdLogs())
	cmd.AddCommand(newCmdMetrics())

	return cmd
}
