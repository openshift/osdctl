package alerts

import (
	"github.com/openshift/osdctl/cmd/alerts/silence"
	"github.com/spf13/cobra"
)

// NewCmdAlerts implements base alert command.
func NewCmdAlerts() *cobra.Command {
	alrtCmd := &cobra.Command{
		Use:               "alert",
		Short:             "List alerts",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	alrtCmd.AddCommand(utils.NewCmdListAlerts())
	alrtCmd.AddCommand(silence.NewCmdSilence())

	return alrtCmd
}
