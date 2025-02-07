package cluster

import (
	"fmt"

	"github.com/openshift/osdctl/cmd/cluster/access"
	"github.com/openshift/osdctl/cmd/cluster/dynatrace"
	"github.com/openshift/osdctl/cmd/cluster/resize"
	"github.com/openshift/osdctl/cmd/cluster/sre_operators"
	"github.com/openshift/osdctl/cmd/cluster/ssh"
	"github.com/openshift/osdctl/cmd/cluster/support"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdCluster implements the cluster utility
func NewCmdCluster(streams genericclioptions.IOStreams, client *k8s.LazyClient, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:               "cluster",
		Short:             "Provides information for a specified cluster",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	clusterCmd.AddCommand(newCmdHealth())
	clusterCmd.AddCommand(newCmdLoggingCheck(streams, globalOpts))
	clusterCmd.AddCommand(newCmdOwner(streams, globalOpts))
	clusterCmd.AddCommand(support.NewCmdSupport(streams, client, globalOpts))
	clusterCmd.AddCommand(resize.NewCmdResize())
	clusterCmd.AddCommand(newCmdResync())
	clusterCmd.AddCommand(newCmdContext())
	clusterCmd.AddCommand(newCmdTransferOwner(streams, globalOpts))
	clusterCmd.AddCommand(access.NewCmdAccess(streams, client))
	clusterCmd.AddCommand(newCmdCpd())
	clusterCmd.AddCommand(newCmdCheckBannedUser())
	clusterCmd.AddCommand(newCmdValidatePullSecret())
	clusterCmd.AddCommand(newCmdEtcdHealthCheck())
	clusterCmd.AddCommand(newCmdEtcdMemberReplacement())
	clusterCmd.AddCommand(newCmdFromInfraId(globalOpts))
	clusterCmd.AddCommand(NewCmdHypershiftInfo(streams))
	clusterCmd.AddCommand(newCmdOrgId())
	clusterCmd.AddCommand(dynatrace.NewCmdDynatrace())
	clusterCmd.AddCommand(newCmdCleanupLeakedEC2())
	clusterCmd.AddCommand(newCmdDetachStuckVolume())
	clusterCmd.AddCommand(ssh.NewCmdSSH())
	clusterCmd.AddCommand(sre_operators.NewCmdSREOperators(streams, client))
	return clusterCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help(): ", err.Error())
	}
}
