package list

import (
	"fmt"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdList implements the list command
func NewCmdList(streams genericclioptions.IOStreams, client client.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	listCmd := &cobra.Command{
		Use:               "list",
		Short:             "List resources",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	listCmd.AddCommand(newCmdListAccount(streams, client, globalOpts))
	listCmd.AddCommand(newCmdListAccountClaim(streams, client, globalOpts))

	return listCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help(): ", err.Error())
	}
}
