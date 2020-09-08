package secret

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	accountIDRequired = "AWS Account ID is required. You can use -i or --account-id to specify it"
)

// NewCmdSecret implements the secret command
func NewCmdSecret(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	secretCmd := &cobra.Command{
		Use:               "secret",
		Short:             "secret <command>",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}
	secretCmd.AddCommand(newCmdCheckSecrets(streams, flags))
	secretCmd.AddCommand(newCmdRotateSecret(streams, flags))
	return secretCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
