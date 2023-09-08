package cluster

import (
	"fmt"

	"github.com/openshift/osdctl/cmd/cluster/access"
	"github.com/openshift/osdctl/cmd/cluster/resize"
	"github.com/openshift/osdctl/cmd/cluster/support"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdCluster implements the cluster utility
func NewCmdCluster(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:               "cluster",
		Short:             "Provides information for a specified cluster",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	clusterCmd.AddCommand(newCmdHealth())
	clusterCmd.AddCommand(newCmdLoggingCheck(streams, flags, globalOpts))
	clusterCmd.AddCommand(newCmdOwner(streams, flags, globalOpts))
	clusterCmd.AddCommand(support.NewCmdSupport(streams, flags, client, globalOpts))
	clusterCmd.AddCommand(resize.NewCmdResize())
	clusterCmd.AddCommand(newCmdContext())
	clusterCmd.AddCommand(newCmdTransferOwner(streams, globalOpts))
	clusterCmd.AddCommand(access.NewCmdAccess(streams, flags))
	clusterCmd.AddCommand(newCmdResizeControlPlaneNode(streams, flags, globalOpts))
	clusterCmd.AddCommand(newCmdCpd())
	clusterCmd.AddCommand(newCmdCheckBannedUser())
	clusterCmd.AddCommand(newCmdValidatePullSecret(client, flags))
	clusterCmd.AddCommand(newCmdEtcdHealthCheck())
	clusterCmd.AddCommand(newCmdEtcdMemberReplacement())
	clusterCmd.AddCommand(newCmdFromInfraId(globalOpts))
	clusterCmd.AddCommand(NewCmdHypershiftInfo(streams))
	return clusterCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help(): ", err.Error())
	}
}
