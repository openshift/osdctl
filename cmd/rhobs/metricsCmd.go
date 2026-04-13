package rhobs

import (
	"fmt"

	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/spf13/cobra"
)

var cmdMetricsOptions = struct {
	clusterId    string
	outputFormat string
}{}

func newCmdMetrics() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics --cluster-id <cluster-identifier> prometheus-expression",
		Short: "Fetch metrics from RHOBS.next",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceErrors = true

			if cmdMetricsOptions.clusterId == "" {
				var err error

				cmdMetricsOptions.clusterId, err = k8s.GetCurrentCluster()
				if err != nil {
					return fmt.Errorf("failed to retrieve ID for current cluster: %v", err)
				}
			}

			if len(args) != 1 {
				return fmt.Errorf("exactly one Prometheus expression must be provided as an argument")
			}

			var outputFormat MetricsFormat
			switch cmdMetricsOptions.outputFormat {
			case string(MetricsFormatTable):
				outputFormat = MetricsFormatTable
			case string(MetricsFormatCsv):
				outputFormat = MetricsFormatCsv
			case string(MetricsFormatJson):
				outputFormat = MetricsFormatJson
			default:
				return fmt.Errorf("invalid output format: %s", cmdMetricsOptions.outputFormat)
			}

			cmd.SilenceUsage = true

			err := printMetrics(cmdMetricsOptions.clusterId, args[0], outputFormat)
			if err != nil {
				return fmt.Errorf("failed to retrieve metrics: %v", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&cmdMetricsOptions.clusterId, "cluster-id", "C", "", "Name or Internal ID of the cluster (defaults to current cluster context)")
	cmd.Flags().StringVarP(&cmdMetricsOptions.outputFormat, "output", "o", string(MetricsFormatTable), "Format of the output (table, csv, json)")

	return cmd
}
