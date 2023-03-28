package aao

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdAao implements the base aao command
func NewCmdAao(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	aaoCmd := &cobra.Command{
		Use:               "aao",
		Short:             "AWS Account Operator Debugging Utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	aaoCmd.AddCommand(newCmdPool(streams, flags, client))

	return aaoCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help()", err.Error())
	}
}
