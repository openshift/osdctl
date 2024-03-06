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

/*
func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help()", err.Error())
	}
}*/

