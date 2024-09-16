package sre_operators

import (
	"github.com/spf13/cobra"
)

func NewCmdSREOperators() *cobra.Command {
	sreOperatorsCmd := &cobra.Command{
		Use:               "sre-operators",
		Short:             "SRE operator related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	sreOperatorsCmd.AddCommand(newCmdList())

	return sreOperatorsCmd
}
