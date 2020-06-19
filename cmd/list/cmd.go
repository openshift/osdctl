package list

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	awsAccountNamespace = "aws-account-operator"
)

// NewCmdList implements the basic list command to list operator crs
func NewCmdList(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	getCmd := &cobra.Command{
		Use:               "list",
		Short:             "list resources",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	getCmd.AddCommand(newCmdListAccount(streams, flags))

	return getCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
