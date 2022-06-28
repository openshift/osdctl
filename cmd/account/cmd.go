package account

import (
	"fmt"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/cmd/account/get"
	"github.com/openshift/osdctl/cmd/account/list"
	"github.com/openshift/osdctl/cmd/account/mgmt"
	"github.com/openshift/osdctl/cmd/account/servicequotas"
	"github.com/openshift/osdctl/internal/utils/globalflags"
)

// NewCmdAccount implements the base account command
func NewCmdAccount(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	accountCmd := &cobra.Command{
		Use:               "account",
		Short:             "AWS Account related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	accountCmd.AddCommand(get.NewCmdGet(streams, flags, client, globalOpts))
	accountCmd.AddCommand(list.NewCmdList(streams, flags, client, globalOpts))
	accountCmd.AddCommand(servicequotas.NewCmdServiceQuotas(streams, flags))
	accountCmd.AddCommand(mgmt.NewCmdMgmt(streams, flags, globalOpts))
	accountCmd.AddCommand(newCmdReset(streams, flags, client))
	accountCmd.AddCommand(newCmdSet(streams, flags, client))
	accountCmd.AddCommand(newCmdConsole(streams, flags))
	accountCmd.AddCommand(newCmdCli(streams, flags, globalOpts))
	accountCmd.AddCommand(newCmdCleanVeleroSnapshots(streams))
	accountCmd.AddCommand(newCmdVerifySecrets(streams, flags, client))
	accountCmd.AddCommand(newCmdRotateSecret(streams, flags, client))
	accountCmd.AddCommand(newCmdGenerateSecret(streams, flags, client))

	return accountCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help(): ", err.Error())
	}
}
