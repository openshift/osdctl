package sre_operators

import (
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewCmdSREOperators(client client.Client) *cobra.Command {
	sreOperatorsCmd := &cobra.Command{
		Use:               "sre-operators",
		Short:             "SRE operator related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	sreOperatorsCmd.AddCommand(newCmdList(client))

	return sreOperatorsCmd
}
