package account

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/osdctl/cmd/account/get"
	"github.com/openshift/osdctl/cmd/account/list"
	"github.com/openshift/osdctl/cmd/account/mgmt"
	"github.com/openshift/osdctl/cmd/account/servicequotas"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/k8s"
)

// NewCmdAccount implements the base account command
func NewCmdAccount(streams genericclioptions.IOStreams, client *k8s.LazyClient, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	accountCmd := &cobra.Command{
		Use:               "account",
		Short:             "AWS Account related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	accountCmd.AddCommand(get.NewCmdGet(streams, client, globalOpts))
	accountCmd.AddCommand(list.NewCmdList(streams, client, globalOpts))
	accountCmd.AddCommand(servicequotas.NewCmdServiceQuotas(streams))
	accountCmd.AddCommand(mgmt.NewCmdMgmt(streams, globalOpts))
	accountCmd.AddCommand(newCmdReset(streams, client))
	accountCmd.AddCommand(newCmdSet(streams, client))
	accountCmd.AddCommand(newCmdConsole())
	accountCmd.AddCommand(newCmdCli())
	accountCmd.AddCommand(newCmdCleanVeleroSnapshots(streams))
	accountCmd.AddCommand(newCmdVerifySecrets(streams, client))
	accountCmd.AddCommand(newCmdRotateSecret(streams, client))
	accountCmd.AddCommand(newCmdGenerateSecret(streams, client))
	accountCmd.AddCommand(newCmdRotateAWSCreds(streams))
	return accountCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help(): ", err.Error())
	}
}
