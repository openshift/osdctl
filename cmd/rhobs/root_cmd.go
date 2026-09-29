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

// NewCmdRhobs builds the RHOBS command and its subcommands.
func NewCmdRhobs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rhobs",
		Short: "RHOBS.next related utilities",
		Long: `RHOBS.next related utilities.

For headless automation, have the trusted launcher inject RHOBS_CLIENT_ID and
RHOBS_CLIENT_SECRET into the osdctl process environment from your secret manager.

Alternatively, set rhobs_client_id and rhobs_client_secret in ~/.config/osdctl:
  rhobs_client_id: "<client-id>"
  rhobs_client_secret: "<client-secret>"
Restrict access to this file (for example, chmod 600 ~/.config/osdctl).

Each environment variable overrides its corresponding config value. An explicitly
empty environment variable returns an error instead of falling back to config.
A complete pair bypasses Vault; an incomplete pair returns an error. Vault is
used only when neither credential is supplied. Direct credentials apply to all
RHOBS environments queried by the process; supply the pair for your target environment.

Keep secret values out of agent prompts, tool calls, and shell tracing.
Environment variables remain accessible to the receiving process and may be
inherited by child processes.`,
		Args: cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			for c := cmd; c != nil; c = c.Parent() {
				if c.Name() == "mcp" {
					return nil
				}
			}
			viper.SetDefault(utils.VaultAddrKey, "https://vault.devshift.net/")
			viper.SetDefault("rhobs_integration_vault_path", "osd-sre/rhobs/sd-sre-integration-creds")
			viper.SetDefault("rhobs_stage_vault_path", "osd-sre/rhobs/sd-sre-stage-creds")
			viper.SetDefault("rhobs_production_vault_path", "osd-sre/rhobs/sd-sre-prod-creds")

			if rhobsCellFlag := cmd.Flags().Lookup("rhobs-cell"); rhobsCellFlag != nil && rhobsCellFlag.Changed && rhobsCellFlag.Value.String() != "" {
				return nil
			}

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
	cmd.AddCommand(newCmdHcpDashboard())
	cmd.AddCommand(newCmdAlerts())
	cmd.AddCommand(newCmdMcp())

	cmd.PersistentFlags().StringVarP(&commonOptions.clusterId, "cluster-id", "C", "", "Name or Internal ID of the cluster (defaults to current cluster context)")
	cmd.PersistentFlags().StringVar(&commonOptions.hiveOcmUrl, "hive-ocm-url", "production", `OCM environment URL for hive operations - aliases: "production", "staging", "integration"`)

	return cmd
}
