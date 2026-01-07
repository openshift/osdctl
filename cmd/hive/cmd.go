package hive

import (
	"fmt"

	cd "github.com/openshift/osdctl/cmd/hive/clusterdeployment"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewCmdHive implements the base hive command
func NewCmdHive(streams genericclioptions.IOStreams, client client.Client) *cobra.Command {
	hiveCmd := &cobra.Command{
		Use:               "hive",
		Short:             "hive related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	hiveCmd.AddCommand(NewCmdClusterSyncFailures(streams, client))
	hiveCmd.AddCommand(cd.NewCmdClusterDeployment(streams, client))
	hiveCmd.AddCommand(newCmdTestHiveLogin())
	return hiveCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while calling cmd.Help()", err.Error())
	}
}
