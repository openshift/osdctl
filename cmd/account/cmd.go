package account

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/osd-utils-cli/cmd/account/get"
)

// NewCmdAccount implements the base account command
func NewCmdAccount(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	accountCmd := &cobra.Command{
		Use:               "account",
		Short:             "AWS Account related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	accountCmd.AddCommand(get.NewCmdGet(streams, flags))
	accountCmd.AddCommand(newCmdReset(streams, flags))
	accountCmd.AddCommand(newCmdSet(streams, flags))
	accountCmd.AddCommand(newCmdConsole(streams, flags))
	accountCmd.AddCommand(newCmdCli(streams, flags))
	accountCmd.AddCommand(newCmdList(streams, flags))
	accountCmd.AddCommand(newCmdCleanVeleroSnapshots(streams))
	accountCmd.AddCommand(newCmdCheckSecrets(streams, flags))
	accountCmd.AddCommand(newCmdRotateSecret(streams, flags))

	return accountCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
