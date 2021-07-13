package mgmt

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmMgmt implements the mgmt command to get AWS Account resources
func NewCmdMgmt(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	mgmtCmd := &cobra.Command{
		Use:               "mgmt",
		Short:             "AWS Account Management",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	mgmtCmd.AddCommand(newCmdAccountList(streams, flags))
	mgmtCmd.AddCommand(newCmdAccountAssign(streams, flags))
	mgmtCmd.AddCommand(newCmdAccountUnassign(streams, flags))

	return mgmtCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
