package rhobs

import (
	"fmt"
	"net/url"
	"strings"

	ocmutils "github.com/openshift/osdctl/pkg/utils"
	"github.com/pkg/browser"
	log "github.com/sirupsen/logrus"
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
				metricsRhobsFetcher, logsRhobsFetcher, err = CreateMetricsAndLogsRhobsFetchers(cmd.Context(), commonOptions.clusterId, commonOptions.hiveOcmUrl)
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

type GrafanaDashboard struct {
	name                 string
	pathId               string
	pathName             string
	validateAndGetParams func(metricsFetcher, logsFetcher *RhobsFetcher) (url.Values, error)
}

const defaultGrafanaDashboardShortName = "hosted-cluster"

func eventuallyWarnAboutWiderDashboardScope(fetcher *RhobsFetcher) {
	if fetcher.clusterId != "" {
		log.Warnf("Dashboard will contain data for clusters other than the provided cluster. "+
			"Works as if the --rhobs-cell option was set to the %s cluster RHOBS cell(s).\n", fetcher.clusterId)
	}
}

var allowedGrafanaDashboard = []*GrafanaDashboard{
	{
		name:     defaultGrafanaDashboardShortName,
		pathId:   "cf6ntunq7rb40c",
		pathName: "rosa-hcp-central-cluster-dashboard",
		validateAndGetParams: func(metricsFetcher, logsFetcher *RhobsFetcher) (url.Values, error) {
			if !metricsFetcher.IsHostedCluster {
				return nil, fmt.Errorf("'%s' dashboard must be used with a hosted cluster", defaultGrafanaDashboardShortName)
			}

			region, shard, err := metricsFetcher.getRhobsRegionAndShard()
			if err != nil {
				return nil, err
			}

			return url.Values{
				"var-environment": {metricsFetcher.ocmEnvName},
				"var-region":      {region},
				"var-shard":       {shard},
				"var-_id":         {metricsFetcher.clusterExternalId},
			}, nil
		},
	}, {
		name:     "management-cluster",
		pathId:   "rosa-hcp-mc-dashboard",
		pathName: "rosa-hcp-management-cluster-dashboard",
		validateAndGetParams: func(metricsFetcher, logsFetcher *RhobsFetcher) (url.Values, error) {
			mcName := metricsFetcher.clusterName

			if !metricsFetcher.isManagementCluster {
				if metricsFetcher.IsHostedCluster {
					managementCluster, err := ocmutils.GetManagementCluster(metricsFetcher.clusterId)
					if err != nil {
						return nil, fmt.Errorf("failed to retrieve management cluster for cluster '%s': %v", metricsFetcher.clusterId, err)
					}
					mcName = managementCluster.Name()
					log.Warnf("Dashboard will contain data for the %s management cluster; not just for the provided hosted cluster: %s", managementCluster.ID(), metricsFetcher.clusterId)
				} else {
					return nil, fmt.Errorf("'%s' dashboard must be used with a management cluster", "management-cluster")
				}
			}

			region, _, err := metricsFetcher.getRhobsRegionAndShard()
			if err != nil {
				return nil, err
			}

			return url.Values{
				"var-environment": {metricsFetcher.ocmEnvName},
				"var-region":      {region},
				"var-mc_name":     {mcName},
			}, nil
		},
	}, {
		name:     "kube-apis-slo",
		pathId:   "cfmgzo0gsak1sd",
		pathName: "drill-down3a-rosa-hcp-api-server-availability",
		validateAndGetParams: func(metricsFetcher, logsFetcher *RhobsFetcher) (url.Values, error) {
			eventuallyWarnAboutWiderDashboardScope(metricsFetcher)

			metricsDataSource, err := metricsFetcher.getMetricsGrafanaDataSource()
			if err != nil {
				return nil, err
			}
			return url.Values{
				"var-environment":       {metricsFetcher.ocmEnvName},
				"var-datasource_global": {metricsDataSource},
			}, nil
		},
	}, {
		name:     "clusters-creation-slo",
		pathId:   "fdmk9z8ucodtsa",
		pathName: "drill-down3a-rosa-hcp-cluster-creation",
		validateAndGetParams: func(metricsFetcher, logsFetcher *RhobsFetcher) (url.Values, error) {
			eventuallyWarnAboutWiderDashboardScope(metricsFetcher)

			region, _, err := metricsFetcher.getRhobsRegionAndShard()
			if err != nil {
				return nil, err
			}

			metricsDataSource, err := metricsFetcher.getMetricsGrafanaDataSource()
			if err != nil {
				return nil, err
			}

			logsDataSource, err := logsFetcher.getLogsGrafanaDataSource()
			if err != nil {
				return nil, err
			}

			return url.Values{
				"var-environment":         {metricsFetcher.ocmEnvName},
				"var-region":              {region},
				"var-datasource_regional": {metricsDataSource},
				"var-datasource_global":   {metricsDataSource},
				"var-datasource_logs":     {logsDataSource},
			}, nil
		},
	}, {
		name:     "control-planes-upgrade-slo",
		pathId:   "efmgzo0i3qmm8d",
		pathName: "drill-down3a-rosa-hcp-control-plane-upgrades",
		validateAndGetParams: func(metricsFetcher, logsFetcher *RhobsFetcher) (url.Values, error) {
			region, _, err := metricsFetcher.getRhobsRegionAndShard()
			if err != nil {
				return nil, err
			}

			metricsDataSource, err := metricsFetcher.getMetricsGrafanaDataSource()
			if err != nil {
				return nil, err
			}

			clusterId := metricsFetcher.clusterId
			if !metricsFetcher.IsHostedCluster {
				clusterId = "$__all"
			}

			return url.Values{
				"var-environment":         {metricsFetcher.ocmEnvName},
				"var-region":              {region},
				"var-datasource_regional": {metricsDataSource},
				"var-datasource_global":   {metricsDataSource},
				"var-clusterid":           {clusterId},
			}, nil
		},
	}, {
		name:     "nodepools-upgrade-slo",
		pathId:   "919c6ec2b6d74bdf",
		pathName: "drill-down3a-rosa-hcp-nodepool-upgrades",
		validateAndGetParams: func(metricsFetcher, logsFetcher *RhobsFetcher) (url.Values, error) {
			metricsDataSource, err := metricsFetcher.getMetricsGrafanaDataSource()
			if err != nil {
				return nil, err
			}

			clusterId := metricsFetcher.clusterId
			if !metricsFetcher.IsHostedCluster {
				clusterId = "$__all"
			}
			mcName := metricsFetcher.clusterName
			if !metricsFetcher.isManagementCluster {
				mcName = "$__all"
			}

			return url.Values{
				"var-datasource":        {metricsDataSource},
				"var-namespace":         {"uhc-" + metricsFetcher.ocmEnvName},
				"var-clusterid":         {clusterId},
				"var-managementcluster": {mcName},
			}, nil
		},
	}, {
		name:     "nodepools-slo",
		pathId:   "cdtg6ugw1a03ka",
		pathName: "drill-down3a-rosa-hcp-nodepools",
		validateAndGetParams: func(metricsFetcher, logsFetcher *RhobsFetcher) (url.Values, error) {
			eventuallyWarnAboutWiderDashboardScope(metricsFetcher)

			region, _, err := metricsFetcher.getRhobsRegionAndShard()
			if err != nil {
				return nil, err
			}

			metricsDataSource, err := metricsFetcher.getMetricsGrafanaDataSource()
			if err != nil {
				return nil, err
			}

			return url.Values{
				"var-environment":         {metricsFetcher.ocmEnvName},
				"var-region":              {region},
				"var-datasource_regional": {metricsDataSource},
			}, nil
		},
	}, {
		name:     "counters",
		pathId:   "bfmgzo0f6uw3kc",
		pathName: "rosa-hcp-counter",
		validateAndGetParams: func(metricsFetcher, logsFetcher *RhobsFetcher) (url.Values, error) {
			eventuallyWarnAboutWiderDashboardScope(metricsFetcher)

			region, _, err := metricsFetcher.getRhobsRegionAndShard()
			if err != nil {
				return nil, err
			}

			metricsDataSource, err := metricsFetcher.getMetricsGrafanaDataSource()
			if err != nil {
				return nil, err
			}

			return url.Values{
				"var-environment":         {metricsFetcher.ocmEnvName},
				"var-region":              {region},
				"var-datasource_regional": {metricsDataSource},
			}, nil
		},
	},
} // Make sure to run `make generate-docs` when editing this list

func GetAllowedGrafanaDashboardsShortNames() []string {
	result := []string{}

	for _, grafanaDashboard := range allowedGrafanaDashboard {
		result = append(result, grafanaDashboard.name)
	}

	return result
}

func GetGrafanaDashboardForShortName(shortName string) *GrafanaDashboard {
	for _, grafanaDashboard := range allowedGrafanaDashboard {
		if grafanaDashboard.name == shortName {
			return grafanaDashboard
		}
	}
	return nil
}

func GetGrafanaDashboardUrl(metricsFetcher, logsFetcher *RhobsFetcher, grafanaDashboard *GrafanaDashboard) (string, error) {
	dashboardParams, err := grafanaDashboard.validateAndGetParams(metricsFetcher, logsFetcher)
	if err != nil {
		return "", fmt.Errorf("failed to get parameters for Grafana dashboard: %v", err)
	}

	return grafanaBaseUrl + "d/" + grafanaDashboard.pathId + "/" + grafanaDashboard.pathName + "?" + dashboardParams.Encode(), nil
}
