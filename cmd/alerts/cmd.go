package alerts

import (
	"github.com/openshift/osdctl/cmd/alerts/silence"
	"github.com/spf13/cobra"
)

func NewCmdAlerts() *cobra.Command {
	alrtCmd := &cobra.Command{
		Use:               "alert",
		Short:             "List alerts",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	alrtCmd.AddCommand(NewCmdListAlerts())
	alrtCmd.AddCommand(silence.NewCmdSilence())

	return alrtCmd
}
