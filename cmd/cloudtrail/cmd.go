package cloudtrail

import (
	"github.com/spf13/cobra"
)

// cloudtrailCmd represents the cloudtrail command
func NewCloudtrailCmd() *cobra.Command {
	cloudtrailCmd := &cobra.Command{
		Use:   "cloudtrail",
		Short: "AWS CloudTrail related utilities",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	cloudtrailCmd.AddCommand(newCmdWriteEvents())

	return cloudtrailCmd
}
