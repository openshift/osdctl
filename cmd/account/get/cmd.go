package get

import (
	"github.com/spf13/cobra"

	"github.com/openshift/osdctl/internal/utils/globalflags"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	accountIDRequired = "AWS Account ID is required. You can use -i or --account-id to specify it"
)

// NewCmdGet implements the get command to get AWS Account related resources
func NewCmdGet(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	getCmd := &cobra.Command{
		Use:               "get",
		Short:             "Get resources",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	getCmd.AddCommand(newCmdGetAccount(streams, flags, globalOpts))
	getCmd.AddCommand(newCmdGetAccountClaim(streams, flags, globalOpts))
	getCmd.AddCommand(newCmdGetLegalEntity(streams, flags))
	getCmd.AddCommand(newCmdGetSecrets(streams, flags))
	getCmd.AddCommand(newCmdGetAWSAccount(streams, flags))

	return getCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
