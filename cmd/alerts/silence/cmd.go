package silence

import (
	"github.com/spf13/cobra"
)

// NewCmdSilence implements base silence command.
func NewCmdSilence() *cobra.Command {
	silenceCmd := &cobra.Command{
		Use:               "silence",
		Short:             "add, expire and list silence associated with alerts",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	silenceCmd.AddCommand(NewCmdAddSilence())
	silenceCmd.AddCommand(NewCmdClearSilence())
	silenceCmd.AddCommand(NewCmdListSilence())

	return silenceCmd
}
