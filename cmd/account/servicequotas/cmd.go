package servicequotas

import (
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

	baseCmd.AddCommand(newCmdDescribe(streams, flags))

	return baseCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
