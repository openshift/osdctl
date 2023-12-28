package alerts

import (
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type alertOptions struct {
	clusterID  string
	level     string
	active    bool
}

func newCmdHealth() *cobra.Command {
	ops := newalertOptions()
	alertCmd := &cobra.Command{
		Use:               "alerts",
		Short:             "Provides alerts related to the cluster",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	alertCmd.Flags().BoolVarP(&ops.active, "active", "", false, "active")
	alertCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Cluster ID")
	alertCmd.Flags().StringVarP(&ops.level, "", "l", "", "level")
	alertCmd.MarkFlagRequired("cluster-id")
	return alertCmd
}

func newalertOptions() *alertOptions {
	return &alertOptions{}
}

func (o *alertOptions) complete(cmd *cobra.Command, _ []string) error {
	return nil
}

