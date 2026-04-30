package rhobs

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newCmdCell() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "cell",
		Short:         "Get the RHOBS cell for a given MC or HCP cluster",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true

			metricsRhobsFetcher, metricsErr := CreateRhobsFetcher(commonOptions.clusterId, RhobsFetchForMetrics, commonOptions.hiveOcmUrl)
			if metricsErr != nil {
				log.Errorf("Error while computing metrics RHOBS cell: %v", metricsErr)
			}

			logsRhobsFetcher, logsErr := CreateRhobsFetcher(commonOptions.clusterId, RhobsFetchForLogs, commonOptions.hiveOcmUrl)
			if logsErr != nil {
				log.Errorf("Error while computing logs RHOBS cell: %v", logsErr)
			}

			if metricsErr == nil {
				if logsErr == nil && metricsRhobsFetcher.RhobsCell == logsRhobsFetcher.RhobsCell {
					fmt.Println("Metrics & logs RHOBS cell:", metricsRhobsFetcher.RhobsCell)
					return
				}
				fmt.Println("Metrics RHOBS cell:", metricsRhobsFetcher.RhobsCell)
			}

			if logsErr == nil {
				fmt.Println("Logs RHOBS cell   :", logsRhobsFetcher.RhobsCell)
			}

			if metricsErr != nil || logsErr != nil {
				os.Exit(1)
			}
		},
	}

	return cmd
}
