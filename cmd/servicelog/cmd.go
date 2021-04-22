package servicelog

import (
	"github.com/spf13/cobra"
)

func NewCmdServiceLog() *cobra.Command {
	var servicelogCmd = &cobra.Command{
		Use:   "servicelog",
		Short: "OCM/Hive Service log",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	// Add subcommands
	servicelogCmd.AddCommand(listCmd) // servicelog list
	servicelogCmd.AddCommand(postCmd) // servicelog post

	return servicelogCmd
}
