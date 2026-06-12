package rhobs

import (
	"fmt"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

func newCmdHcpDashboard() *cobra.Command {
	var rhobsCell string
	var isOpeningGrafanaUrl bool
	var dashboardName string

	cmd := &cobra.Command{
		Use:   "hcp-dashboard [dashboard-name]",
		Short: "Get the HCP dashboard URL for a given HCP cluster",
		Long: "Get the HCP dashboard URL for a given HCP cluster. " +
			"The dashboard name is optional and defaults to the hosted cluster dashboard. " +
			"Allowed values for the dashboard name are: " + strings.Join(GetAllowedGrafanaDashboardsShortNames(), ", ") + ". " +
			"The URL of the RHOBS cell(s) to use can be specified with the --rhobs-cell option, " +
			"but it is usually more convenient to specify the cluster with the --cluster-id option and let the command figure out the RHOBS cell(s) to use. " +
			"Note that the --rhobs-cell option is not working with all dashboards and cannot be used together with the --cluster-id option.",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dashboardName = defaultGrafanaDashboardShortName
			if len(args) == 1 {
				dashboardName = args[0]
			}

			grafanaDashboard := GetGrafanaDashboardForShortName(dashboardName)
			if grafanaDashboard == nil {
				return fmt.Errorf("invalid dashboard name: %s", dashboardName)
			}

			rhobsCells := []string{}
			if rhobsCell != "" {
				if cmd.Parent().Flags().Changed("cluster-id") {
					return fmt.Errorf("--rhobs-cell and --cluster-id options cannot be used together")
				}

				rhobsCells = strings.Split(rhobsCell, ",")
				if len(rhobsCells) > 2 {
					return fmt.Errorf("value passed to --rhobs-cell must have at most 2 elements: %s", rhobsCell)
				}
			}

			cmd.SilenceUsage = true

			var metricsRhobsFetcher, logsRhobsFetcher *RhobsFetcher
			var err error

			if len(rhobsCells) > 0 {
				metricsRhobsFetcher, err = CreateRhobsFetcherFromCell(rhobsCells[0])
				if err != nil {
					return err
				}
				if len(rhobsCells) == 2 {
					logsRhobsFetcher, err = CreateRhobsFetcherFromCell(rhobsCells[1])
					if err != nil {
						return err
					}
				} else {
					logsRhobsFetcher = metricsRhobsFetcher
				}
			} else {
				metricsRhobsFetcher, logsRhobsFetcher, err = CreateMetricsAndLogsRhobsFetchers(commonOptions.clusterId, commonOptions.hiveOcmUrl)
				if err != nil {
					return err
				}
			}

			grafanaUrl, err := GetGrafanaDashboardUrl(metricsRhobsFetcher, logsRhobsFetcher, grafanaDashboard)
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

			return nil
		},
	}

	cmd.Flags().StringVarP(&rhobsCell, "rhobs-cell", "c", "", "RHOBS cell URL - "+
		"for instance: https://us-east-1-0.rhobs.api.stage.openshift.com - "+
		"use a comma to separate the RHOBS cell to use for metrics from the logs RHOBS cell if they are different - "+
		"this option is not working with all dashboards - exclusive with --cluster-id")
	cmd.Flags().BoolVarP(&isOpeningGrafanaUrl, "browser", "b", false, "Open in the URL in the default browser")

	return cmd
}
