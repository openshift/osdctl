package org

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewCmdOrg() *cobra.Command {
	var orgCmd = &cobra.Command{
		Use:   "org",
		Short: "Provides information for a specified organization",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			if err != nil {
				fmt.Println("Error calling cmd.Help(): ", err.Error())
				return
			}
		},
	}

	// Add subcommands

	orgCmd.AddCommand(currentCmd)
	orgCmd.AddCommand(getCmd)
	orgCmd.AddCommand(usersCmd)
	orgCmd.AddCommand(labelsCmd)
	orgCmd.AddCommand(describeCmd)
	orgCmd.AddCommand(clustersCmd)
	orgCmd.AddCommand(customersCmd)
	orgCmd.AddCommand(awsAccountsCmd)
	orgCmd.AddCommand(contextCmd)

	return orgCmd
}
