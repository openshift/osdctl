package cluster

import (
	"fmt"

	"github.com/openshift/osdctl/cmd/cluster/access"
	"github.com/openshift/osdctl/cmd/cluster/support"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	k8spkg "github.com/openshift/osdctl/pkg/k8s"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
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

	clusterCmd.AddCommand(newCmdHealth(streams, flags, globalOpts))
	clusterCmd.AddCommand(newCmdLoggingCheck(streams, flags, globalOpts))
	clusterCmd.AddCommand(newCmdOwner(streams, flags, globalOpts))
	clusterCmd.AddCommand(support.NewCmdSupport(streams, flags, client, globalOpts))
	clusterCmd.AddCommand(newCmdContext(streams, flags, globalOpts))
	clusterCmd.AddCommand(newCmdTransferOwner(streams, flags, globalOpts))
	clusterCmd.AddCommand(access.NewCmdAccess(streams, flags))
	clusterCmd.AddCommand(newCmdResizeControlPlaneNode(streams, flags, globalOpts))
	return clusterCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help(): ", err.Error())
	}
}

func CompleteValidation(o *k8spkg.ClusterResourceFactoryOptions, io genericclioptions.IOStreams) error {
	k8svalid, err := o.ValidateIdentifiers()
	if !k8svalid {
		if err != nil {
			cmdutil.PrintErrorWithCauses(err, io.ErrOut)
			return err
		}

	}

	awsvalid, err := o.Awscloudfactory.ValidateIdentifiers()
	if !awsvalid {
		if err != nil {
			return err
		}
	}
	return nil
}
