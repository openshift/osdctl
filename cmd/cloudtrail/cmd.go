package cloudtrail

import (
	"github.com/spf13/cobra"
)

// NewCloudtrailCmd represents the newCmdWriteEvents command
func NewCloudtrailCmd() *cobra.Command {
	cloudtrailCmd := &cobra.Command{
		Use:   "cloudtrail",
		Short: "AWS CloudTrail related utilities",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cloudtrailCmd.AddCommand(newCmdWriteEvents())
	cloudtrailCmd.AddCommand(newCmdPermissionDenied())

	return cloudtrailCmd
}
