package dynatrace

import (
	"github.com/spf13/cobra"
)

const (
	DynatraceTenantKeyLabel    string = "sre-capabilities.dtp.tenant"
	HypershiftClusterTypeLabel string = "ext-hypershift.openshift.io/cluster-type"
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
	dtCmd.AddCommand(NewCmdHCPLogs())

	return dtCmd
}
