package mgmt

import (
	"fmt"

	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	osdStaging1          = "osd-staging-1"
	osdStaging2          = "osd-staging-2"
	envKeyAWSAccountName = "AWS_ACCOUNT_NAME"
)

// NewCmdMgmt implements the mgmt command to get AWS Account resources
func NewCmdMgmt(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	mgmtCmd := &cobra.Command{
		Use:               "mgmt",
		Short:             "AWS Account Management",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	mgmtCmd.AddCommand(newCmdAccountList(streams, globalOpts))
	mgmtCmd.AddCommand(newCmdAccountAssign(streams, globalOpts))
	mgmtCmd.AddCommand(newCmdAccountUnassign(streams))
	mgmtCmd.AddCommand(newCmdAccountIAM(streams, globalOpts))

	return mgmtCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help(): ", err.Error())
		return
	}
}
