package alerts

import (
	"fmt"
	"github.com/spf13/cobra"
)

func NewCmdAlerts() *cobra.Command {
	alrtCmd := &cobra.Command{
		Use:               "alerts",
		Short:             "list alerts",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	alrtCmd.AddCommand(NewCmdListAlerts())
	alrtCmd.AddCommand(NewCmdAddSilence())
	alrtCmd.AddCommand(NewCmdClearSilence())
	alrtCmd.AddCommand(NewCmdListSilence())

	return alrtCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help()", err.Error())
	}
}
