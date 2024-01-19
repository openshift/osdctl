package cloudtrail

import (
	"github.com/spf13/cobra"
)

var cloudtrailCmd = &cobra.Command{
	Use:   "cloudtrail",
	Short: "cloudtrail is a palette that contains cloudtrail commands",
	Long:  `cloudtrail is a palette that contains cloudtrail commands`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	cloudtrailCmd.AddCommand(newwrite_eventsCmd())
}
