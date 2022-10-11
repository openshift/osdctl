package servicequotas

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdServiceQuotas implements commands related to AWS service-quotas
func NewCmdServiceQuotas(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	baseCmd := &cobra.Command{
		Use:               "servicequotas",
		Short:             "Interact with AWS service-quotas",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Aliases:           []string{"service-quotas", "service-quota"},
	}

	baseCmd.AddCommand(newCmdDescribe())

	return baseCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help()", err.Error())
	}
}
