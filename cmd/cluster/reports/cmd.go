package reports

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewCmdReports implements the reports command to list, get, and create cluster reports
// osdctl cluster reports list --cluster-id <cluster-id>
// osdctl cluster reports get --cluster-id <cluster-id> --report-id <report-id>
// osdctl cluster reports create --cluster-id <cluster-id> --summary <summary> --data <data>
// osdctl cluster reports create --cluster-id <cluster-id> --summary <summary> --file <file-path>
func NewCmdReports() *cobra.Command {
	reportsCmd := &cobra.Command{
		Use:   "reports",
		Short: "Manage cluster reports in backplane-api",
		Long: `Manage cluster reports stored in backplane-api.

Cluster reports are used to store and retrieve diagnostic information
and other data related to cluster operations. Reports are associated with a
specific cluster and include a summary and base64-encoded data.`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	reportsCmd.AddCommand(newCmdList())
	reportsCmd.AddCommand(newCmdGet())
	reportsCmd.AddCommand(newCmdCreate())

	return reportsCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("error in reports command: ", err.Error())
		return
	}
}
