package cloudtrail

import (
	"github.com/spf13/cobra"
)

// cloudtrailCmd represents the cloudtrail command
func NewCloudtrailCmd() *cobra.Command {
	cloudtrailCmd := &cobra.Command{
		Use:   "cloudtrail",
		Short: "cloudtrail is a palette that contains cloudtrail commands",
		Long:  `cloudtrail is a palette that contains cloudtrail commands`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	cloudtrailCmd.AddCommand(newwrite_eventsCmd())

	return cloudtrailCmd
}
