package cloudtrail

import (
	"github.com/spf13/cobra"
)

// If *cobra.command is empty, return man page for Write and PermissionDenied (?)

// NewCloudtrailCmd represents the newCmdWriteEvents command

func NewCloudtrailCmd() *cobra.Command { //returns pointer for cobra.Command
	//Known as composite literals
	//cobra.Command is a struct that consist of Use:string, Short:string and Run -> runs a function
	// & allows you to set values of the keys in cobra.Command
	cloudtrailCmd := &cobra.Command{
		Use:   "cloudtrail",
		Short: "AWS CloudTrail related utilities",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	// Adds new writeEvents or PermissionDenied to both files respectively
	cloudtrailCmd.AddCommand(newCmdWriteEvents())
	cloudtrailCmd.AddCommand(newCmdPermissionDenied())

	return cloudtrailCmd
}
