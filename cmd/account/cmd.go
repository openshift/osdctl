package account

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/cmd/account/get"
	"github.com/openshift/osdctl/cmd/account/list"
	"github.com/openshift/osdctl/cmd/account/mgmt"
	"github.com/openshift/osdctl/cmd/account/servicequotas"
)

// NewCmdAccount implements the base account command
func NewCmdAccount(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	accountCmd := &cobra.Command{
		Use:               "account",
		Short:             "AWS Account related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	accountCmd.AddCommand(get.NewCmdGet(streams, flags, client))
	accountCmd.AddCommand(list.NewCmdList(streams, flags, client))
	accountCmd.AddCommand(servicequotas.NewCmdServiceQuotas(streams, flags))
	accountCmd.AddCommand(mgmt.NewCmdMgmt(streams, flags))
	accountCmd.AddCommand(newCmdReset(streams, flags, client))
	accountCmd.AddCommand(newCmdSet(streams, flags, client))
	accountCmd.AddCommand(newCmdConsole(streams, flags))
	accountCmd.AddCommand(newCmdCli(streams, flags))
	accountCmd.AddCommand(newCmdCleanVeleroSnapshots(streams))
	accountCmd.AddCommand(newCmdVerifySecrets(streams, flags, client))
	accountCmd.AddCommand(newCmdRotateSecret(streams, flags, client))
	accountCmd.AddCommand(newCmdGenerateSecret(streams, flags, client))

	return accountCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
