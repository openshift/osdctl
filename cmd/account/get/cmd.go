package get

import (
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/internal/utils/globalflags"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	accountIDRequired = "AWS Account ID is required. You can use -i or --account-id to specify it"
)

// NewCmdGet implements the get command to get AWS Account related resources
func NewCmdGet(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	getCmd := &cobra.Command{
		Use:               "get",
		Short:             "Get resources",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run:               help,
	}

	getCmd.AddCommand(newCmdGetAccount(streams, flags, client, globalOpts))
	getCmd.AddCommand(newCmdGetAccountClaim(streams, flags, client, globalOpts))
	getCmd.AddCommand(newCmdGetLegalEntity(streams, flags, client, globalOpts))
	getCmd.AddCommand(newCmdGetSecrets(streams, flags, client, globalOpts))
	getCmd.AddCommand(newCmdGetAWSAccount(streams, flags, client))

	return getCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
