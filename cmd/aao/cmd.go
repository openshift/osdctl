package aao

import (
	"fmt"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdAao implements the base aao command
func NewCmdAao(client client.Client) *cobra.Command {
	aaoCmd := &cobra.Command{
		Use:               "aao",
		Short:             "AWS Account Operator Debugging Utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	aaoCmd.AddCommand(newCmdPool(client))

	return aaoCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help()", err.Error())
	}
}
