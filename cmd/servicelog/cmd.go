package servicelog

import (
	"fmt"
	"github.com/spf13/cobra"
)

func NewCmdServiceLog() *cobra.Command {
	var servicelogCmd = &cobra.Command{
		Use:   "servicelog",
		Short: "OCM/Hive Service log",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			if err != nil {
				fmt.Println("Error calling cmd.Help(): ", err.Error())
				return
			}
		},
	}

	// Add subcommands
	servicelogCmd.AddCommand(listCmd)      // servicelog list
	servicelogCmd.AddCommand(newPostCmd()) // servicelog post

	return servicelogCmd
}
