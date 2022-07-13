package mgmt

import (
	"fmt"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdMgmt implements the mgmt command to get AWS Account resources
func NewCmdMgmt(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	mgmtCmd := &cobra.Command{
		Use:               "mgmt",
		Short:             "AWS Account Management",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	mgmtCmd.AddCommand(newCmdAccountList(streams, flags, globalOpts))
	mgmtCmd.AddCommand(newCmdAccountAssign(streams, flags, globalOpts))
	mgmtCmd.AddCommand(newCmdAccountUnassign(streams, flags))

	return mgmtCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help(): ", err.Error())
		return
	}
}
