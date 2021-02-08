package cmd

import (
	"github.com/spf13/cobra"
)

// servicelogCmd represents the servicelog command
var servicelogCmd = &cobra.Command{
	Use:   "servicelog",
	Short: "OCM/Hive Service log",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}
