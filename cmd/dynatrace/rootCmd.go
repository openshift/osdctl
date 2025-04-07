package dynatrace

import (
	"github.com/spf13/cobra"
)

func NewCmdDynatrace() *cobra.Command {
	dtCmd := &cobra.Command{
		Use:               "dynatrace",
		Aliases:           []string{"dt"},
		Short:             "Dynatrace related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	dtCmd.AddCommand(NewCmdLogs())
	dtCmd.AddCommand(newCmdURL())
	dtCmd.AddCommand(newCmdDashboard())
	dtCmd.AddCommand(NewCmdHCPMustGather())

	return dtCmd
}
