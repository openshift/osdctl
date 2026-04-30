package rhobs

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCmdMetrics() *cobra.Command {
	var outputFormatStr string
	var isPrintingClusterResultsOnly bool

	cmd := &cobra.Command{
		Use:           "metrics prometheus-expression",
		Short:         "Fetch metrics from RHOBS",
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("exactly one Prometheus expression must be provided as an argument")
			}

			outputFormat, err := GetMetricsFormatFromString(outputFormatStr)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			rhobsFetcher, err := CreateRhobsFetcher(commonOptions.clusterId, RhobsFetchForMetrics, commonOptions.hiveOcmUrl)
			if err != nil {
				return err
			}

			err = rhobsFetcher.PrintMetrics(args[0], outputFormat, isPrintingClusterResultsOnly)
			if err != nil {
				return fmt.Errorf("failed to print metrics: %v", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFormatStr, "output", "o", string(MetricsFormatTable), "Format of the output - allowed values: \"table\", \"csv\" or \"json\"")
	cmd.Flags().BoolVarP(&isPrintingClusterResultsOnly, "filter", "f", false, "Only keep the results matching the given cluster - "+
		"only effective if some of those results have a _id, _mc_id or mc_name label")

	return cmd
}
