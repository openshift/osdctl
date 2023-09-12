package support

import (
	"fmt"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdSupport implements the get command to get support status
// osdctl cluster support status
// osdctl cluster support create --summary="" --reason=""
// osdctl cluster support delete --reason=""
func NewCmdSupport(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	supportCmd := &cobra.Command{
		Use:               "support",
		Short:             "Cluster Support",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	supportCmd.AddCommand(newCmdstatus(streams, globalOpts))
	supportCmd.AddCommand(newCmdpost())
	supportCmd.AddCommand(newCmddelete(streams, globalOpts))

	return supportCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("error in support command: ", err.Error())
		return
	}
}
