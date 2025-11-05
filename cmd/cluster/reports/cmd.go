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
		Use:               "reports",
		Short:             "Cluster Reports from backplane-api",
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
