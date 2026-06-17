package rhobs

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newCmdCell() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cell",
		Short: "Get the RHOBS cell for a given cluster",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true

			metricsRhobsFetcher, logsRhobsFetcher, err := CreateMetricsAndLogsRhobsFetchers(commonOptions.clusterId, commonOptions.hiveOcmUrl)
			if err != nil {
				log.Errorf("Error while computing RHOBS cells: %v", err)
				os.Exit(1)
			}

			if metricsRhobsFetcher != nil {
				if logsRhobsFetcher != nil && metricsRhobsFetcher.RhobsCell == logsRhobsFetcher.RhobsCell {
					fmt.Println("Metrics & logs RHOBS cell:", metricsRhobsFetcher.RhobsCell)
					return
				}

				fmt.Println("Metrics RHOBS cell:", metricsRhobsFetcher.RhobsCell)
			}

			if logsRhobsFetcher != nil {
				fmt.Println("Logs RHOBS cell   :", logsRhobsFetcher.RhobsCell)
			}
		},
	}

	return cmd
}
