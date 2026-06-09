package rhobs

import (
	"fmt"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

func newCmdMetrics() *cobra.Command {
	var isComputingGrafanaUrl bool
	var isOpeningGrafanaUrl bool
	var evalTime time.Time
	var startTime time.Time
	var endTime time.Time
	var duration time.Duration
	var stepDuration time.Duration
	var outputFormatStr string
	var isPrintingClusterResultsOnly bool

	cmd := &cobra.Command{
		Use:   "metrics [PromQL-expression]",
		Short: "Fetch metrics from RHOBS for a given cluster",
		Long: "Fetch metrics from RHOBS for a given cluster. " +
			"The cluster can be a hosted cluster (HCP), a management cluster (MC) or whatever cluster sending metrics to RHOBS. " +
			"The prometheus expression provided as an argument can be either an instant query or a range query; it is optional if the --url option is set. " +
			"By default, the command will try to evaluate the expression as an instant query at the current time, " +
			"but it is possible to specify a different evaluation time using the --time option or a time range using the --start-time, --end-time and --since options. " +
			"Results can be filtered to only keep the ones matching the given cluster (--cluster-id option) with the --filter option " +
			"even if it is more efficient to do that filtering at the prometheus expression level.",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isComputingGrafanaUrl {
				if isOpeningGrafanaUrl {
					return fmt.Errorf("--browser can only be set if --url is set")
				}

				if len(args) != 1 {
					return fmt.Errorf("a Prometheus expression must be provided when --url is not set")
				}
			}

			nowTime := time.Now()
			defaultStartTime := nowTime.Add(-30 * time.Minute)

			if cmd.Flags().Changed("since") {
				if duration <= 0 {
					return fmt.Errorf("--since must be greater than 0")
				}
				startTime = nowTime.Add(-duration)
			} else if !cmd.Flags().Changed("start-time") {
				startTime = defaultStartTime
			}
			if cmd.Flags().Changed("end-time") {
				if !cmd.Flags().Changed("start-time") && !cmd.Flags().Changed("url") {
					return fmt.Errorf("--end-time can only be set if --start-time or --url is set")
				}
			} else {
				endTime = nowTime
			}
			if startTime.After(endTime) {
				return fmt.Errorf("value passed to --start-time must be before the value passed to --end-time")
			}
			if cmd.Flags().Changed("step") {
				if !cmd.Flags().Changed("start-time") && !cmd.Flags().Changed("since") && !cmd.Flags().Changed("url") {
					return fmt.Errorf("--step can only be set if --start-time, --since or --url is set")
				}
				if stepDuration <= 0 {
					return fmt.Errorf("--step must be greater than 0")
				}
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

			if isComputingGrafanaUrl {
				promExpr := ""
				if len(args) == 1 {
					promExpr = args[0]
				}
				grafanaUrl, err := rhobsFetcher.GetGrafanaMetricsUrl(promExpr, startTime, endTime)
				if err != nil {
					return fmt.Errorf("failed to compute Grafana URL: %v", err)
				}
				if isOpeningGrafanaUrl {
					err = browser.OpenURL(grafanaUrl)
					if err != nil {
						return fmt.Errorf("failed to open Grafana URL in browser: %v", err)
					}
				} else {
					fmt.Println(grafanaUrl)
				}
			} else {
				if cmd.Flags().Changed("since") || cmd.Flags().Changed("start-time") {
					err = rhobsFetcher.PrintRangeMetrics(cmd.Context(), args[0], NewMetricsTimeRange(startTime, endTime, stepDuration), outputFormat, isPrintingClusterResultsOnly)
				} else {
					err = rhobsFetcher.PrintInstantMetrics(cmd.Context(), args[0], evalTime, outputFormat, isPrintingClusterResultsOnly)
				}
				if err != nil {
					return fmt.Errorf("failed to print metrics: %v", err)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&isComputingGrafanaUrl, "url", "u", false, "Only compute and print the grafana URL")
	cmd.Flags().BoolVarP(&isOpeningGrafanaUrl, "browser", "b", false, "Open in the default browser the URL computed with the --url option - only applicable if --url is set")

	cmd.Flags().TimeVar(&evalTime, "time", time.Time{}, []string{time.RFC3339}, "Time at which the PromQL expression must be evaluated - exclusive with --url (default to now)")
	cmd.MarkFlagsMutuallyExclusive("time", "url")
	cmd.Flags().TimeVar(&startTime, "start-time", time.Time{}, []string{time.RFC3339}, "Start time at which the PromQL expression must be evaluated - enable time range mode - exclusive with --time (default to 30 minutes ago)")
	cmd.Flags().TimeVar(&endTime, "end-time", time.Time{}, []string{time.RFC3339}, "End time at which the PromQL expression must be evaluated - can only be set if --start-time or --url is set (default to now)")
	cmd.Flags().DurationVar(&duration, "since", 0, "Only return values newer than a relative duration (e.g. 1h, 30m) - enable time range mode - exclusive with --time, --start-time & --end-time")
	cmd.Flags().DurationVar(&stepDuration, "step", 0, "Duration between data points (e.g. 30s, 2m) - can only be set if in time range mode (i.e. --start-time or --since is set)")
	cmd.MarkFlagsMutuallyExclusive("time", "start-time", "since")
	cmd.MarkFlagsMutuallyExclusive("time", "end-time", "since")

	cmd.Flags().StringVarP(&outputFormatStr, "output", "o", string(MetricsFormatTable), `Format of the output - allowed values: "table", "csv" or "json" - exclusive with --url`)
	cmd.MarkFlagsMutuallyExclusive("output", "url")
	cmd.Flags().BoolVarP(&isPrintingClusterResultsOnly, "filter", "f", false, "Only keep the results matching the given cluster - "+
		"only effective if some of those results have a _id, _mc_id or mc_name label - exclusive with --url")
	cmd.MarkFlagsMutuallyExclusive("filter", "url")

	return cmd
}
