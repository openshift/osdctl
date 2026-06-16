package rhobs

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCmdCell() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "cell",
		Short:         "Get the RHOBS cell for a given cluster",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			metricsRhobsFetcher, logsRhobsFetcher, err := CreateMetricsAndLogsRhobsFetchers(commonOptions.clusterId, commonOptions.hiveOcmUrl)
			if metricsRhobsFetcher != nil {
				if logsRhobsFetcher != nil && metricsRhobsFetcher.RhobsCell == logsRhobsFetcher.RhobsCell {
					fmt.Println("Metrics & logs RHOBS cell:", metricsRhobsFetcher.RhobsCell)
					return nil
				}

				fmt.Println("Metrics RHOBS cell:", metricsRhobsFetcher.RhobsCell)
			}

			if logsRhobsFetcher != nil {
				fmt.Println("Logs RHOBS cell   :", logsRhobsFetcher.RhobsCell)
			}

			return err
		},
	}

	return cmd
}
