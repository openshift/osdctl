package org

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewCmdOrg() *cobra.Command {
	var orgCmd = &cobra.Command{
		Use:   "org",
		Short: "OCM/Org",
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
	orgCmd.AddCommand(lablesCmd)

	return orgCmd
}
