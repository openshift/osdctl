package get

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	accountIDRequired = "AWS Account ID is required. You can use -i or --account-id to specify it"
)

// NewCmdGet implements the get command to get AWS Account related resources
func NewCmdGet(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	getCmd := &cobra.Command{
		Use:               "get",
		Short:             "get resources",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	getCmd.AddCommand(newCmdGetAccount(streams, flags))
	getCmd.AddCommand(newCmdGetAccountClaim(streams, flags))
	getCmd.AddCommand(newCmdGetLegalEntity(streams, flags))
	getCmd.AddCommand(newCmdGetSecrets(streams, flags))
	getCmd.AddCommand(newCmdGetAWSAccount(streams, flags))

	return getCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
