package cluster

import (
	"github.com/openshift/osdctl/cmd/cluster/support"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	k8spkg "github.com/openshift/osdctl/pkg/k8s"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdCluster implements the base cluster health command
func NewCmdCluster(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:               "cluster",
		Short:             "Provides information for a specified cluster",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	clusterCmd.AddCommand(newCmdHealth(streams, flags, globalOpts))
	clusterCmd.AddCommand(newCmdloggingCheck(streams, flags, globalOpts))
	clusterCmd.AddCommand(newCmdOwner(streams, flags, globalOpts))
	clusterCmd.AddCommand(support.NewCmdSupport(streams, flags, client, globalOpts))
	clusterCmd.AddCommand(newCmdcontext(streams, flags, globalOpts))

	return clusterCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
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
