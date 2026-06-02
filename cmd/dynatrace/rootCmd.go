package dynatrace

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewCmdDynatrace() *cobra.Command {
	dtCmd := &cobra.Command{
		Use:               "dynatrace",
		Aliases:           []string{"dt"},
		Short:             "Dynatrace related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Deprecated:        "use 'osdctl rhobs' instead",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: 'osdctl dynatrace' (alias 'dt') is deprecated and will be removed in a future release. Please use 'osdctl rhobs' instead.\n\n")
		},
	}

	dtCmd.AddCommand(NewCmdLogs())
	dtCmd.AddCommand(newCmdURL())
	dtCmd.AddCommand(newCmdDashboard())
	dtCmd.AddCommand(NewCmdHCPMustGather())

	return dtCmd
}
