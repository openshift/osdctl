package promote

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/osdctl/cmd/promote/pko"
	"github.com/openshift/osdctl/cmd/promote/saas"
	"github.com/openshift/osdctl/internal/utils/globalflags"
)

// NewCmdPromote implements the promote command to promote services/operators
func NewCmdPromote(flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	promoteCmd := &cobra.Command{
		Use:               "promote",
		Short:             "Utilities to promote services/operators",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	promoteCmd.AddCommand(saas.NewCmdSaas(flags, globalOpts))
	promoteCmd.AddCommand(pko.NewCmdPKO(flags, globalOpts))

	return promoteCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help(): ", err.Error())
	}
}
