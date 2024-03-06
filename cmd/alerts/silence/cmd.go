package silence

import (
	"github.com/spf13/cobra"
)

func NewCmdSilence() *cobra.Command {
	silenceCmd := &cobra.Command{
		Use:               "silence",
		Short:             "add,clear and list silence",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	silenceCmd.AddCommand(NewCmdAddSilence())
	silenceCmd.AddCommand(NewCmdClearSilence())
	silenceCmd.AddCommand(NewCmdListSilence())

	return silenceCmd
}

