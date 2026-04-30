package rhobs

import (
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/spf13/cobra"
)

var commonOptions = struct {
	clusterId  string
	hiveOcmUrl string
}{}

func NewCmdRhobs() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "rhobs",
		Short:         "RHOBS.next related utilities",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if commonOptions.clusterId == "" {
				var err error

				commonOptions.clusterId, err = k8s.GetCurrentCluster()
				if err != nil {
					return err
				}
			}

			return nil
		},
	}

	cmd.AddCommand(newCmdCell())
	cmd.AddCommand(newCmdLogs())
	cmd.AddCommand(newCmdMetrics())

	cmd.PersistentFlags().StringVarP(&commonOptions.clusterId, "cluster-id", "C", "", "Name or Internal ID of the cluster (defaults to current cluster context)")
	cmd.PersistentFlags().StringVar(&commonOptions.hiveOcmUrl, "hive-ocm-url", "production", "OCM environment URL for hive operations - aliases: \"production\", \"staging\", \"integration\"")

	return cmd
}
