package promote

import (
	"fmt"

	"github.com/openshift/osdctl/cmd/promote/pko"
	"github.com/openshift/osdctl/cmd/promote/saas"
	"github.com/spf13/cobra"
)

// NewCmdPromote implements the promote command to promote services/operators
func NewCmdPromote() *cobra.Command {
	promoteCmd := &cobra.Command{
		Use:               "promote",
		Short:             "Utilities to promote services/operators",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	promoteCmd.AddCommand(saas.NewCmdSaas())
	promoteCmd.AddCommand(pko.NewCmdPKO())

	return promoteCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help(): ", err.Error())
	}
}
