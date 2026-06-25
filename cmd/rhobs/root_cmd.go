package rhobs

import (
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var commonOptions = struct {
	clusterId  string
	hiveOcmUrl string
}{}

func NewCmdRhobs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rhobs",
		Short: "RHOBS.next related utilities",
		Args:  cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			for c := cmd; c != nil; c = c.Parent() {
				if c.Name() == "mcp" {
					return nil
				}
			}
			if commonOptions.clusterId == "" {
				var err error

				commonOptions.clusterId, err = k8s.GetCurrentCluster()
				if err != nil {
					return err
				}
			}

			// Default config
			viper.SetDefault(utils.VaultAddrKey, "https://vault.devshift.net/")
			viper.SetDefault("rhobs_integration_vault_path", "osd-sre/rhobs/sd-sre-integration-creds")
			viper.SetDefault("rhobs_stage_vault_path", "osd-sre/rhobs/sd-sre-stage-creds")
			viper.SetDefault("rhobs_production_vault_path", "osd-sre/rhobs/sd-sre-prod-creds")

			return nil
		},
	}

	cmd.AddCommand(newCmdCell())
	cmd.AddCommand(newCmdLogs())
	cmd.AddCommand(newCmdMetrics())
	cmd.AddCommand(newCmdHcpDashboard())
	cmd.AddCommand(newCmdAlerts())
	cmd.AddCommand(newCmdMcp())

	cmd.PersistentFlags().StringVarP(&commonOptions.clusterId, "cluster-id", "C", "", "Name or Internal ID of the cluster (defaults to current cluster context)")
	cmd.PersistentFlags().StringVar(&commonOptions.hiveOcmUrl, "hive-ocm-url", "production", `OCM environment URL for hive operations - aliases: "production", "staging", "integration"`)

	return cmd
}
