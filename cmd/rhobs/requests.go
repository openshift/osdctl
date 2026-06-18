package rhobs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	ocmutils "github.com/openshift/osdctl/pkg/utils"

	"github.com/cenkalti/backoff/v4"
	"github.com/gorilla/websocket"
	rhobsclient "github.com/observatorium/api/client"
	rhobsparameters "github.com/observatorium/api/client/parameters"
	sdk "github.com/openshift-online/ocm-sdk-go"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	authUrl                   = "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token"
	rhobsVaultPathKeyTemplate = "rhobs_%s_vault_path"
	clusterIdCdLabel          = "api.openshift.com/id"
	rhobsCellCdLabel          = "ext-hypershift.openshift.io/rhobs-cell"
	rhobsCellMetricsCmNs      = "openshift-observability-operator"
	rhobsCellMetricsCmName    = "rhobs-metrics-destination"
	rhobsCellLogsCmNs         = "openshift-logging"
	rhobsCellLogsCmName       = "rhobs-logs-destination"
	rhobsCellCmAnnotation     = "rhobs.openshift.io/forwarding-destination"
	grafanaBaseUrl            = "https://grafana.app-sre.devshift.net/"
)

type RhobsFetchUsage string

const (
	RhobsFetchForMetrics RhobsFetchUsage = "metrics"
	RhobsFetchForLogs    RhobsFetchUsage = "logs"
)

type MetricsFormat string

const (
	MetricsFormatTable MetricsFormat = "table"
	MetricsFormatCsv   MetricsFormat = "csv"
	MetricsFormatJson  MetricsFormat = "json"
)

func GetMetricsFormatFromString(formatStr string) (MetricsFormat, error) {
	switch formatStr {
	case string(MetricsFormatTable):
		return MetricsFormatTable, nil
	case string(MetricsFormatCsv):
		return MetricsFormatCsv, nil
	case string(MetricsFormatJson):
		return MetricsFormatJson, nil
	default:
		return MetricsFormatTable, fmt.Errorf("invalid output format: %s", formatStr)
	}
}

type LogsFormat string

const (
	LogsFormatText LogsFormat = "text"
	LogsFormatCsv  LogsFormat = "csv"
	LogsFormatJson LogsFormat = "json"
)

func GetLogsFormatFromString(formatStr string) (LogsFormat, error) {
	switch formatStr {
	case string(LogsFormatText):
		return LogsFormatText, nil
	case string(LogsFormatCsv):
		return LogsFormatCsv, nil
	case string(LogsFormatJson):
		return LogsFormatJson, nil
	default:
		return LogsFormatText, fmt.Errorf("invalid output format: %s", formatStr)
	}
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

type RhobsFetcher struct {
	clusterId           string
	clusterExternalId   string
	clusterName         string
	IsHostedCluster     bool
	isManagementCluster bool
	ocmEnvName          string
	RhobsCell           string
	tokenProvider       ocmutils.AccessTokenProvider
}

func getRhobsCellFromConfigMap(ctx context.Context, clusterId string, ocmConn *sdk.Connection, configMapNamespace, configMapName, configMapAnnotationKey string) (string, error) {
	client, err := k8s.NewWithConn(clusterId, client.Options{}, ocmConn)
	if err != nil {
		return "", fmt.Errorf("failed to create kube client for cluster '%s': %v", clusterId, err)
	}

	var configMap corev1.ConfigMap

	err = client.Get(ctx, types.NamespacedName{Namespace: configMapNamespace, Name: configMapName}, &configMap)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve config map named '%s' in namespace '%s' for cluster '%s': %v", configMapName, configMapNamespace, clusterId, err)
	}

	rhobsCell, ok := configMap.Annotations[configMapAnnotationKey]
	if !ok {
		return "", fmt.Errorf("config map named '%s' in namespace '%s' for cluster '%s' does not have the following annotation key: %s", configMapName, configMapNamespace, clusterId, configMapAnnotationKey)
	}

	return rhobsCell, nil
}

func getFallbackUsage(usage RhobsFetchUsage) RhobsFetchUsage {
	if usage == RhobsFetchForMetrics {
		return RhobsFetchForLogs
	} else {
		return RhobsFetchForMetrics
	}
}

func getRhobsCellFromHiveClusterDeployment(ctx context.Context, clusterId string, ocmConn *sdk.Connection, hiveOcmUrl string) (string, error) {
	hiveOcmConn, err := ocmutils.CreateConnectionWithUrl(hiveOcmUrl)
	if err != nil {
		return "", err
	}
	defer hiveOcmConn.Close()

	hiveCluster, err := ocmutils.GetHiveClusterWithConn(clusterId, ocmConn, hiveOcmConn)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve hive cluster for cluster '%s': %v", clusterId, err)
	}

	hiveClient, err := k8s.NewWithConn(hiveCluster.ID(), client.Options{}, hiveOcmConn)
	if err != nil {
		return "", fmt.Errorf("failed to create kube client for hive cluster '%s': %v", hiveCluster.ID(), err)
	}

	clusterSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: map[string]string{
		clusterIdCdLabel: clusterId,
	}})
	if err != nil {
		return "", fmt.Errorf("failed to create label selector for cluster '%s': %v", clusterId, err)
	}

	var clusterDeployments hivev1.ClusterDeploymentList

	err = hiveClient.List(ctx, &clusterDeployments, &client.ListOptions{LabelSelector: clusterSelector})
	if err != nil {
		return "", fmt.Errorf("failed to list cluster deployments for cluster '%s': %v", clusterId, err)
	}

	if len(clusterDeployments.Items) != 1 {
		return "", fmt.Errorf("expected to find exactly 1 cluster deployment for cluster '%s', but found %d", clusterId, len(clusterDeployments.Items))
	}

	clusterDeployment := clusterDeployments.Items[0]
	rhobsCell, ok := clusterDeployment.Labels[rhobsCellCdLabel]
	if !ok {
		return "", fmt.Errorf("cluster deployment for cluster '%s' does not have the following label: %s", clusterId, rhobsCellCdLabel)
	}

	return "https://" + rhobsCell, nil
}

func populateRhobsFetchers(ctx context.Context, clusterKey string, hiveOcmUrl string, usageToFetcher *map[RhobsFetchUsage]*RhobsFetcher) error {
	ocmConn, err := ocmutils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocmConn.Close()

	cluster, err := ocmutils.GetCluster(ocmConn, clusterKey)
	if err != nil {
		return err
	}

	var monitoredClusterId string
	isHcp := cluster.Hypershift().Enabled()
	isMC := false

	if isHcp {
		managementCluster, err := ocmutils.GetManagementCluster(cluster.ID())
		if err != nil {
			return fmt.Errorf("failed to retrieve management cluster for cluster '%s': %v", cluster.ID(), err)
		}
		monitoredClusterId = managementCluster.ID()
		log.Infof("Cluster %s is managed by MC cluster %s - using the MC cluster for RHOBS cell resolution\n", cluster.ID(), monitoredClusterId)
	} else {
		monitoredClusterId = cluster.ID()
		isMC, err = ocmutils.IsManagementCluster(cluster.ID())
		if err != nil {
			return fmt.Errorf("failed to determine if cluster '%s' is a management cluster: %v", cluster.ID(), err)
		}
	}

	usageToRhobsCell := map[RhobsFetchUsage]*string{}

	getRhobsCell := func(usage RhobsFetchUsage) string {
		rhobsCellPtr := usageToRhobsCell[usage]
		if rhobsCellPtr != nil {
			return *rhobsCellPtr
		}

		var cmNs, cmName, rhobsCell string
		var err error

		switch usage {
		case RhobsFetchForMetrics:
			cmNs, cmName = rhobsCellMetricsCmNs, rhobsCellMetricsCmName
		case RhobsFetchForLogs:
			cmNs, cmName = rhobsCellLogsCmNs, rhobsCellLogsCmName
		}

		if usage == "CD" {
			rhobsCell, err = getRhobsCellFromHiveClusterDeployment(ctx, monitoredClusterId, ocmConn, hiveOcmUrl)

			if err != nil {
				log.Warnf("Failed to get RHOBS cell from hive cluster deployment for cluster '%s': %v\n", monitoredClusterId, err)
			}
		} else {
			rhobsCell, err = getRhobsCellFromConfigMap(ctx, monitoredClusterId, ocmConn, cmNs, cmName, rhobsCellCmAnnotation)

			if err != nil {
				log.Warnf("Failed to get RHOBS cell from %s config map in %s namespace for cluster '%s': %v\n", cmName, cmNs, monitoredClusterId, err)
				log.Infoln("Trying to get the RHOBS cell from the hive cluster deployment instead...")
			}
		}

		usageToRhobsCell[usage] = &rhobsCell // Can be the empty string in case of error

		return rhobsCell
	}

	resolveRhobsCell := func(usage RhobsFetchUsage) (string, error) { // Must be called for either the metrics or the logs usage
		rhobsCell := getRhobsCell(usage)

		if rhobsCell == "" {
			log.Infoln("Trying to get the RHOBS cell from the hive cluster deployment instead...")
			rhobsCell = getRhobsCell("CD")
		}

		if rhobsCell == "" {
			log.Infoln("Still failing - trying to get the RHOBS cell from another config map instead...")
			fallbackUsage := getFallbackUsage(usage)
			if fallbackUsage != "" {
				rhobsCell = getRhobsCell(fallbackUsage)
			}
		}

		if rhobsCell == "" {
			return "", fmt.Errorf("failed to get RHOBS cell for cluster '%s' despite trying all possible methods", monitoredClusterId)
		}

		return rhobsCell, nil
	}

	baseFetcher := RhobsFetcher{
		clusterId:           cluster.ID(),
		clusterExternalId:   cluster.ExternalID(),
		clusterName:         cluster.Name(),
		IsHostedCluster:     isHcp,
		isManagementCluster: isMC,
		ocmEnvName:          ocmutils.GetCurrentOCMEnv(ocmConn),
	}

	var resolveErr error

	for usage := range *usageToFetcher {
		rhobsCell, err := resolveRhobsCell(usage)
		if err != nil {
			resolveErr = err
			continue
		}

		fetcher := baseFetcher
		fetcher.RhobsCell = rhobsCell
		(*usageToFetcher)[usage] = &fetcher
	}

	// We have to overwrite the fact that backplane just mangled our configuration.
	// TODO: Do not use the global configuration instead (https://issues.redhat.com/browse/OSD-19773)
	err = osdctlConfig.EnsureConfigFile()
	if err != nil {
		log.Warnf("failed to reload osdctl config: %v", err)
	}

	return resolveErr
}

func CreateMetricsAndLogsRhobsFetchers(ctx context.Context, clusterKey string, hiveOcmUrl string) (*RhobsFetcher, *RhobsFetcher, error) {
	usageToFetcher := map[RhobsFetchUsage]*RhobsFetcher{
		RhobsFetchForMetrics: nil,
		RhobsFetchForLogs:    nil,
	}
	err := populateRhobsFetchers(ctx, clusterKey, hiveOcmUrl, &usageToFetcher)

	return usageToFetcher[RhobsFetchForMetrics], usageToFetcher[RhobsFetchForLogs], err
}

func CreateRhobsFetcher(ctx context.Context, clusterKey string, usage RhobsFetchUsage, hiveOcmUrl string) (*RhobsFetcher, error) {
	usageToFetcher := map[RhobsFetchUsage]*RhobsFetcher{
		usage: nil,
	}
	err := populateRhobsFetchers(ctx, clusterKey, hiveOcmUrl, &usageToFetcher)

	return usageToFetcher[usage], err
}

func CreateRhobsFetcherFromCell(rhobsCell string) (*RhobsFetcher, error) {
	envName := ""

	if strings.HasSuffix(rhobsCell, ".rhobs.api.openshift.com") {
		envName = "production"
	} else {
		for _, currentEnvName := range []string{"stage", "integration"} {
			if strings.HasSuffix(rhobsCell, ".rhobs.api."+currentEnvName+".openshift.com") {
				envName = currentEnvName
			}
		}
	}

	if envName == "" {
		return nil, fmt.Errorf("failed to determine OCM environment from RHOBS cell URL '%s'", rhobsCell)
	}

	return &RhobsFetcher{
		ocmEnvName: envName,
		RhobsCell:  rhobsCell,
	}, nil
}

func (f *RhobsFetcher) getRhobsCellName() (string, error) {
	re := regexp.MustCompile("https://([^.]+).*")
	matches := re.FindStringSubmatch(f.RhobsCell)
	if len(matches) < 2 {
		return "", fmt.Errorf("failed to extract RHOBS cell name from '%s'", f.RhobsCell)
	}

	return matches[1], nil
}

func (f *RhobsFetcher) getRhobsRegionAndShard() (string, string, error) {
	rhobsCellName, err := f.getRhobsCellName()
	lastDashIdx := strings.LastIndex(rhobsCellName, "-")
	if err != nil || lastDashIdx == -1 {
		return "", "", fmt.Errorf("failed to extract RHOBS cell region & shard from '%s'", f.RhobsCell)
	}

	return rhobsCellName[:lastDashIdx], rhobsCellName[lastDashIdx+1:], nil
}

func (f *RhobsFetcher) getBaseGrafanaDataSource() (string, error) {
	rhobsCellName, err := f.getRhobsCellName()
	if err != nil {
		return "", fmt.Errorf("failed to compute grafana datasource: %v", err)
	}

	return "rhobs-" + rhobsCellName + "-" + f.ocmEnvName + "-hcp-", nil
}

func (f *RhobsFetcher) getMetricsGrafanaDataSource() (string, error) {
	baseDataSource, err := f.getBaseGrafanaDataSource()
	if err != nil {
		return "", err
	}

	return baseDataSource + "metrics", nil
}

func (f *RhobsFetcher) getLogsGrafanaDataSource() (string, error) {
	baseDataSource, err := f.getBaseGrafanaDataSource()
	if err != nil {
		return "", err
	}

	return baseDataSource + "logs", nil
}

type grafanaMetricsExploreParams struct {
	Panel struct {
		DataSource string `json:"datasource"`
		Queries    [1]struct {
			RefId        string `json:"refId"`
			Expr         string `json:"expr"`
			Instant      bool   `json:"instant"`
			Range        bool   `json:"range"`
			EditorMode   string `json:"editorMode"`
			LegendFormat string `json:"legendFormat"`
		} `json:"queries"`
		Range struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"range"`
	} `json:"aaa"`
}

type grafanaLogsExploreParams struct {
	Panel struct {
		DataSource string `json:"datasource"`
		Queries    [1]struct {
			RefId      string `json:"refId"`
			Expr       string `json:"expr"`
			QueryType  string `json:"queryType"`
			EditorMode string `json:"editorMode"`
			Direction  string `json:"direction"`
		} `json:"queries"`
		Range struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"range"`
	} `json:"aaa"`
}

func (f *RhobsFetcher) GetGrafanaMetricsUrl(promExpr string, startTime, endTime time.Time) (string, error) {
	exploreParams := grafanaMetricsExploreParams{}

	dataSource, err := f.getMetricsGrafanaDataSource()
	if err != nil {
		return "", err
	}
	exploreParams.Panel.DataSource = dataSource
	exploreParams.Panel.Queries[0].RefId = "osdctl"
	exploreParams.Panel.Queries[0].Expr = promExpr
	exploreParams.Panel.Queries[0].Instant = true
	exploreParams.Panel.Queries[0].Range = true
	exploreParams.Panel.Queries[0].EditorMode = "code"
	exploreParams.Panel.Queries[0].LegendFormat = "__auto"
	exploreParams.Panel.Range.From = strconv.FormatInt(startTime.UnixMilli(), 10)
	exploreParams.Panel.Range.To = strconv.FormatInt(endTime.UnixMilli(), 10)

	exploreParamsBytes, err := json.Marshal(exploreParams)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Grafana explore parameters: %v", err)
	}

	exploreParamsEncoded := url.Values{
		"schemaVersion": {"1"},
		"panes":         {string(exploreParamsBytes)},
	}.Encode()

	return grafanaBaseUrl + "explore?" + exploreParamsEncoded, nil
}

func (f *RhobsFetcher) GetGrafanaLogsUrl(lokiExpr string, startTime, endTime time.Time, isGoingForward bool) (string, error) {
	exploreParams := grafanaLogsExploreParams{}

	dataSource, err := f.getLogsGrafanaDataSource()
	if err != nil {
		return "", err
	}

	var logsDir string

	if isGoingForward {
		logsDir = "forward"
	} else {
		logsDir = "backward"
	}

	exploreParams.Panel.DataSource = dataSource
	exploreParams.Panel.Queries[0].RefId = "osdctl"
	exploreParams.Panel.Queries[0].Expr = lokiExpr
	exploreParams.Panel.Queries[0].QueryType = "range"
	exploreParams.Panel.Queries[0].EditorMode = "code"
	exploreParams.Panel.Queries[0].Direction = logsDir
	exploreParams.Panel.Range.From = strconv.FormatInt(startTime.UnixMilli(), 10)
	exploreParams.Panel.Range.To = strconv.FormatInt(endTime.UnixMilli(), 10)

	exploreParamsBytes, err := json.Marshal(exploreParams)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Grafana explore parameters: %v", err)
	}

	exploreParamsEncoded := url.Values{
		"schemaVersion": {"1"},
		"panes":         {string(exploreParamsBytes)},
	}.Encode()

	return grafanaBaseUrl + "explore?" + exploreParamsEncoded, nil
}

func GetGrafanaDashboardUrl(metricsFetcher, logsFetcher *RhobsFetcher, grafanaDashboard *GrafanaDashboard) (string, error) {
	dashboardParams, err := grafanaDashboard.validateAndGetParams(metricsFetcher, logsFetcher)
	if err != nil {
		return "", fmt.Errorf("failed to get parameters for Grafana dashboard: %v", err)
	}

	return grafanaBaseUrl + "d/" + grafanaDashboard.pathId + "/" + grafanaDashboard.pathName + "?" + dashboardParams.Encode(), nil
}

func (f *RhobsFetcher) getTokenProvider() (ocmutils.AccessTokenProvider, error) {
	if f.tokenProvider == nil {
		tokenProvider, err := ocmutils.GetScopedTokenProvider(authUrl, fmt.Sprintf(rhobsVaultPathKeyTemplate, f.ocmEnvName), "profile")
		if err != nil {
			return nil, fmt.Errorf("failed to get access token provider: %v", err)
		}
		f.tokenProvider = tokenProvider
	}
	return f.tokenProvider, nil
}

func (f *RhobsFetcher) getClient() (*rhobsclient.ClientWithResponses, error) {
	tokenProvider, err := f.getTokenProvider()
	if err != nil {
		return nil, err
	}

	rhobsClient, err := rhobsclient.NewClientWithResponses(f.RhobsCell, func(rhobsClient *rhobsclient.Client) error {
		rhobsClient.RequestEditors = append(rhobsClient.RequestEditors, func(ctx context.Context, req *http.Request) error {
			accessToken, err := tokenProvider.Token()
			if err != nil {
				return fmt.Errorf("failed to get access token: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+accessToken)
			return nil
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create RHOBS client: %v", err)
	}
	return rhobsClient, nil
}

type jsonInterceptor[T any] struct {
	raw     []byte // Used by json format - preserve undecoded fields
	decoded T
}

func (w *jsonInterceptor[T]) UnmarshalJSON(raw []byte) error {
	w.raw = raw
	return json.Unmarshal(raw, &w.decoded)
}

func (w *jsonInterceptor[T]) MarshalJSON() ([]byte, error) {
	return w.raw, nil
}

type getMetricsResponse[metricResult instantMetricResult | rangeMetricResult] struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string                           `json:"resultType"`
		Results    []*jsonInterceptor[metricResult] `json:"result"`
	} `json:"data"`
}

type instantMetricResult struct {
	Metric map[string]string `json:"metric"`
	Value  metricData        `json:"value"`
	// No native histograms support just yet but can be seen using json format
}

type rangeMetricResult struct {
	Metric map[string]string `json:"metric"`
	Values []metricData      `json:"values"`
	// No native histograms support just yet but can be seen using json format
}

type metricData []interface{}

func (d *metricData) isValid() bool {
	return len(*d) == 2
}

func (d *metricData) getHumanReadableTime() string {
	if len(*d) < 1 {
		return ""
	}

	tsFloat, ok := (*d)[0].(float64)
	if !ok {
		return ""
	}
	return time.Unix(int64(tsFloat), 0).String()
}

func (d *metricData) getMachineReadableTime() string {
	if len(*d) < 1 {
		return ""
	}

	return fmt.Sprintf("%.3f", (*d)[0])
}

func (d *metricData) getValue() string {
	if len(*d) < 2 {
		return ""
	}

	return (*d)[1].(string)
}

type timestampFormatter func(d *metricData) string

type metricsTableColumn struct {
	name  string
	width int
}

func getMetricsTableColumns(results *[]*jsonInterceptor[instantMetricResult], tsFormatter timestampFormatter) []metricsTableColumn {
	timeColumn := metricsTableColumn{name: "TIME", width: len("TIME")}
	valueColumn := metricsTableColumn{name: "VALUE", width: len("VALUE")}
	labelNameToColumn := make(map[string]*metricsTableColumn)
	labelNames := []string{} // to maintain order of label columns

	for _, result := range *results {
		if !result.decoded.Value.isValid() {
			continue
		}
		time := tsFormatter(&result.decoded.Value)
		if len(time) > timeColumn.width {
			timeColumn.width = len(time)
		}
		value := result.decoded.Value.getValue()
		if len(value) > valueColumn.width {
			valueColumn.width = len(value)
		}

		for labelName, labelValue := range result.decoded.Metric {
			if _, exists := labelNameToColumn[labelName]; !exists {
				labelNameToColumn[labelName] = &metricsTableColumn{name: labelName, width: len(labelName)}
				labelNames = append(labelNames, labelName)
			}
			if len(labelValue) > labelNameToColumn[labelName].width {
				labelNameToColumn[labelName].width = len(labelValue)
			}
		}
	}

	columns := []metricsTableColumn{timeColumn, valueColumn}

	sort.Strings(labelNames)
	for _, labelName := range labelNames {
		columns = append(columns, *labelNameToColumn[labelName])
	}

	return columns
}

type instantMetricsPrinter func(*[]*jsonInterceptor[instantMetricResult])

func printMetricsAsTable(results *[]*jsonInterceptor[instantMetricResult]) {
	columns := getMetricsTableColumns(results, func(d *metricData) string { return d.getHumanReadableTime() })
	separatorLine := "+"
	for _, column := range columns {
		separatorLine += strings.Repeat("-", column.width+2) + "+"
	}

	fmt.Println(separatorLine)

	// Header
	fmt.Print("|")
	for _, column := range columns {
		fmt.Printf(" %-*s |", column.width, column.name)
	}
	fmt.Println()
	fmt.Println(separatorLine)

	// Rows
	for _, result := range *results {
		if !result.decoded.Value.isValid() {
			continue
		}
		time := result.decoded.Value.getHumanReadableTime()
		value := result.decoded.Value.getValue()
		fmt.Printf("| %*s | %*s |", columns[0].width, time, columns[1].width, value)

		for _, column := range columns[2:] {
			labelValue := result.decoded.Metric[column.name]
			fmt.Printf(" %*s |", column.width, labelValue)
		}
		fmt.Println()
	}
	fmt.Println(separatorLine)
}

func printMetricsAsCsv(results *[]*jsonInterceptor[instantMetricResult]) {
	columns := getMetricsTableColumns(results, func(d *metricData) string { return d.getMachineReadableTime() })

	writer := csv.NewWriter(os.Stdout)

	header := []string{}
	for _, column := range columns {
		header = append(header, column.name)
	}
	err := writer.Write(header)
	if err != nil {
		log.Warnln("Failed to write CSV header:", err)
		return
	}

	for _, result := range *results {
		if !result.decoded.Value.isValid() {
			continue
		}
		row := []string{
			result.decoded.Value.getMachineReadableTime(),
			result.decoded.Value.getValue(),
		}
		for _, column := range columns[2:] {
			row = append(row, result.decoded.Metric[column.name])
		}
		err := writer.Write(row)
		if err != nil {
			log.Warnln("Failed to write CSV row:", err)
			return
		}
	}

	writer.Flush()
}

func printMetricsAsJson[metricResult instantMetricResult | rangeMetricResult](results *[]*jsonInterceptor[metricResult]) {
	metricsBytes, err := json.MarshalIndent(results, "", "  ")
	if err == nil {
		fmt.Println(string(metricsBytes))
	} else {
		log.Warnln("Failed to marshal metrics data:", err)
	}
}

func createInstantMetricsPrinter(format MetricsFormat) instantMetricsPrinter {
	switch format {
	case MetricsFormatCsv:
		return printMetricsAsCsv
	case MetricsFormatJson:
		return printMetricsAsJson[instantMetricResult]
	default:
		return printMetricsAsTable
	}
}

type instantOrRangeMetricResult interface {
	instantMetricResult | rangeMetricResult

	getLabel(labelKey string) (string, bool)
}

func (result instantMetricResult) getLabel(labelKey string) (string, bool) {
	value, exists := result.Metric[labelKey]
	return value, exists
}

func (result rangeMetricResult) getLabel(labelKey string) (string, bool) {
	value, exists := result.Metric[labelKey]
	return value, exists
}

func filterMetricsResults[metricResult instantOrRangeMetricResult](fetcher *RhobsFetcher, results *[]*jsonInterceptor[metricResult], isPrintingClusterResultsOnly bool) *[]*jsonInterceptor[metricResult] {
	areSomeResultsForOtherClusters := false // If true, this means areFilteredResultsValid is also true
	filteredResults := []*jsonInterceptor[metricResult]{}
	areFilteredResultsValid := len(*results) == 0

	for _, result := range *results {
		if fetcher.isManagementCluster {
			mcId, hasMcId := result.decoded.getLabel("_mc_id")
			mcName, hasMcName := result.decoded.getLabel("mc_name")
			if !hasMcId && !hasMcName {
				continue
			}
			areFilteredResultsValid = true
			if mcId != fetcher.clusterId && mcName != fetcher.clusterName {
				areSomeResultsForOtherClusters = mcId != "" || mcName != ""
				continue
			}
		} else {
			extId, hasExtId := result.decoded.getLabel("_id")
			if !hasExtId {
				continue
			}
			areFilteredResultsValid = true
			if extId != fetcher.clusterExternalId {
				areSomeResultsForOtherClusters = extId != ""
				continue
			}
		}
		filteredResults = append(filteredResults, result)
	}

	var returnedResults *[]*jsonInterceptor[metricResult]

	if isPrintingClusterResultsOnly && areFilteredResultsValid {
		returnedResults = &filteredResults
	} else {
		if isPrintingClusterResultsOnly {
			log.Warnln("Results returned by RHOBS cannot be matched against the provided cluster. Working as if --filter option was not set.")
		}
		returnedResults = results
		if !isPrintingClusterResultsOnly && areSomeResultsForOtherClusters {
			log.Warnln("Printing ALL RHOBS cell results even the ones not matching the provided cluster. " +
				"You could have used the --filter option to only print the results matching the provided cluster.")
		} else {
			log.Infoln("Printing ALL RHOBS cell results")
		}
	}

	return returnedResults
}

func (f *RhobsFetcher) queryInstantMetrics(ctx context.Context, promExpr string, evalTime time.Time) (*[]*jsonInterceptor[instantMetricResult], error) {
	client, err := f.getClient()
	if err != nil {
		return nil, err
	}

	log.Infoln("RHOBS cell:", f.RhobsCell)

	promQuery := rhobsparameters.PromqlQuery(promExpr)
	var evalTimePStr *string
	if !evalTime.IsZero() {
		evalTimeStr := strconv.FormatInt(evalTime.Unix(), 10)
		evalTimePStr = &evalTimeStr
	}

	queryParams := &rhobsclient.GetInstantQueryParams{
		Query: &promQuery,
		Time:  evalTimePStr,
	}

	response, err := client.GetInstantQueryWithResponse(ctx, "hcp", queryParams)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to RHOBS: %v", err)
	}
	if response.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RHOBS query failed with status code: %d - body: %s", response.HTTPResponse.StatusCode, string(response.Body))
	}

	var formattedResponse getMetricsResponse[instantMetricResult]
	if err := json.Unmarshal(response.Body, &formattedResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response from RHOBS: %v", err)
	}
	if formattedResponse.Status != "success" {
		return nil, fmt.Errorf("RHOBS query failed with status: %s", formattedResponse.Status)
	}

	return &formattedResponse.Data.Results, nil
}

func (f *RhobsFetcher) PrintInstantMetrics(ctx context.Context, promExpr string, evalTime time.Time, format MetricsFormat, isPrintingClusterResultsOnly bool) error {
	results, err := f.queryInstantMetrics(ctx, promExpr, evalTime)
	if err != nil {
		return err
	}

	results = filterMetricsResults(f, results, isPrintingClusterResultsOnly)
	createInstantMetricsPrinter(format)(results)

	return nil
}

type MetricsTimeRange struct {
	rawStartTime    string
	rawEndTime      string
	rawStepDuration string
	logCallback     func()
}

func (tr *MetricsTimeRange) Log() {
	if tr.logCallback != nil {
		tr.logCallback()
	}
}

func NewMetricsTimeRange(startTime, endTime time.Time, stepDuration time.Duration) MetricsTimeRange {
	if stepDuration == 0 {
		stepDurationTarget := endTime.Sub(startTime) / 1000
		stepDuration = stepDurationTarget
		for _, stepDurationCandidate := range []time.Duration{
			30 * time.Second,
			time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 30 * time.Minute,
			time.Hour, 2 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour,
		} {
			if stepDurationCandidate >= stepDurationTarget {
				stepDuration = stepDurationCandidate
				break
			}
		}
	}

	return MetricsTimeRange{
		rawStartTime:    strconv.FormatInt(startTime.Unix(), 10),
		rawEndTime:      strconv.FormatInt(endTime.Unix(), 10),
		rawStepDuration: strconv.FormatFloat(stepDuration.Seconds(), 'f', -1, 64),
		logCallback: func() {
			log.Infoln("Start time:", startTime.Round(0))
			log.Infoln("End time  :", endTime.Round(0))
			log.Infoln("Step      :", stepDuration)
		},
	}
}

func newRawMetricsTimeRange(start, end, step string) MetricsTimeRange {
	return MetricsTimeRange{
		rawStartTime:    start,
		rawEndTime:      end,
		rawStepDuration: step,
	}
}

func (f *RhobsFetcher) queryRangeMetrics(ctx context.Context, promExpr string, timeRange MetricsTimeRange) (*[]*jsonInterceptor[rangeMetricResult], error) {
	client, err := f.getClient()
	if err != nil {
		return nil, err
	}

	log.Infoln("RHOBS cell:", f.RhobsCell)
	timeRange.Log()

	promQuery := rhobsparameters.PromqlQuery(promExpr)

	queryParams := &rhobsclient.GetRangeQueryParams{
		Query: &promQuery,
		Start: (*rhobsparameters.StartTS)(&timeRange.rawStartTime),
		End:   (*rhobsparameters.EndTS)(&timeRange.rawEndTime),
		Step:  &timeRange.rawStepDuration,
	}

	response, err := client.GetRangeQueryWithResponse(ctx, "hcp", queryParams)

	if err != nil {
		return nil, fmt.Errorf("failed to send request to RHOBS: %v", err)
	}
	if response.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RHOBS query failed with status code: %d - body: %s", response.HTTPResponse.StatusCode, string(response.Body))
	}

	var formattedResponse getMetricsResponse[rangeMetricResult]
	err = json.Unmarshal(response.Body, &formattedResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response from RHOBS: %v", err)
	}

	if formattedResponse.Status != "success" {
		return nil, fmt.Errorf("RHOBS query failed with status: %s", formattedResponse.Status)
	}

	return &formattedResponse.Data.Results, nil
}

func (f *RhobsFetcher) PrintRangeMetrics(ctx context.Context, promExpr string, timeRange MetricsTimeRange, format MetricsFormat, isPrintingClusterResultsOnly bool) error {
	results, err := f.queryRangeMetrics(ctx, promExpr, timeRange)
	if err != nil {
		return err
	}

	results = filterMetricsResults(f, results, isPrintingClusterResultsOnly)

	if format == MetricsFormatJson {
		printMetricsAsJson[rangeMetricResult](results)
		return nil
	}

	instantResults := []*jsonInterceptor[instantMetricResult]{}
	for k := range *results {
		result := &(*results)[k]
		for l := range (*result).decoded.Values {
			instantResults = append(instantResults, &jsonInterceptor[instantMetricResult]{
				// 'raw' field not defined - that's ok we are not gonna encode json
				decoded: instantMetricResult{
					Metric: (*result).decoded.Metric,
					Value:  (*result).decoded.Values[l],
				},
			})
		}
	}

	createInstantMetricsPrinter(format)(&instantResults)

	return nil
}

type getLogsResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string       `json:"resultType"`
		Results    []*logResult `json:"result"`
	} `json:"data"`
}

type streamLogsResponse struct {
	Streams []*logResult `json:"streams"`
}

type logResult struct {
	Stream    *map[string]string `json:"stream"`
	Values    []*[]string        `json:"values"`
	timeStamp int64              `json:"-"`
}

func (l *logResult) getTimeStamp() int64 {
	if l.timeStamp == 0 {
		l.timeStamp = -1
		values := l.Values[0]
		if len(*values) == 0 {
			log.Warnln("Log entry does not have a timestamp")
			return -1
		}

		ts, err := strconv.ParseInt((*values)[0], 10, 64)
		if err != nil {
			log.Warnf("Error parsing timestamp '%s' as an integer: %v", (*values)[0], err)
			return -1
		}
		l.timeStamp = ts
	}
	return l.timeStamp
}

func (l *logResult) getTime() time.Time {
	return time.Unix(0, l.getTimeStamp())
}

func (l *logResult) getMessage() string {
	values := l.Values[0]
	if len(*values) < 2 {
		log.Warnln("No message available for log entry")
		return ""
	}

	return (*values)[1]
}

type logsPrinter interface {
	PrintHeader()
	PrintResult(result *logResult)
	PrintTrailer()
}

type textLogsPrinter struct {
	isPrintingTimeValue bool
	fieldNames          []string
}

func (p *textLogsPrinter) PrintHeader() {
}

func (p *textLogsPrinter) PrintResult(result *logResult) {
	var sb strings.Builder

	if p.isPrintingTimeValue {
		sb.WriteString(result.getTime().String())
		sb.WriteString(" ")
	}
	for _, fieldName := range p.fieldNames {
		sb.WriteString((*result.Stream)[fieldName])
		sb.WriteString(" ")
	}
	sb.WriteString(result.getMessage())

	fmt.Println(sb.String())
}

func (p *textLogsPrinter) PrintTrailer() {
}

type csvLogsPrinter struct {
	writer              *csv.Writer
	isPrintingTimeValue bool
	fieldNames          []string
}

func (p *csvLogsPrinter) PrintHeader() {
	header := []string{}
	if p.isPrintingTimeValue {
		header = append(header, "TIME")
	}
	header = append(header, p.fieldNames...)
	header = append(header, "MESSAGE")

	err := p.writer.Write(header)
	if err != nil {
		log.Warnln("Failed to write CSV header:", err)
		return
	}
}

func (p *csvLogsPrinter) PrintResult(result *logResult) {
	row := []string{}
	if p.isPrintingTimeValue {
		row = append(row, result.getTime().String())
	}
	for _, fieldName := range p.fieldNames {
		row = append(row, (*result.Stream)[fieldName])
	}
	row = append(row, result.getMessage())

	err := p.writer.Write(row)
	if err != nil {
		log.Warnln("Failed to write CSV row:", err)
	}
}

func (p *csvLogsPrinter) PrintTrailer() {
	p.writer.Flush()
}

type jsonLogsPrinter struct {
	isFirstLogPrinted bool
}

func (p *jsonLogsPrinter) PrintHeader() {
	fmt.Print("[")
}

func (p *jsonLogsPrinter) PrintResult(result *logResult) {
	if p.isFirstLogPrinted {
		fmt.Print(",")
	}
	fmt.Println()

	logBytes, err := json.MarshalIndent(result, "  ", "  ")
	if err != nil {
		log.Warnln("Error marshaling json:", err)
		return
	}
	fmt.Print("  ")
	fmt.Print(string(logBytes))

	p.isFirstLogPrinted = true
}

func (p *jsonLogsPrinter) PrintTrailer() {
	fmt.Println()
	fmt.Println("]")
}

func createLogsPrinter(format LogsFormat, isPrintingTimeValue bool, fieldNames []string) logsPrinter {
	switch format {
	case LogsFormatCsv:
		return &csvLogsPrinter{writer: csv.NewWriter(os.Stdout), isPrintingTimeValue: isPrintingTimeValue, fieldNames: fieldNames}
	case LogsFormatJson:
		return &jsonLogsPrinter{}
	default:
		return &textLogsPrinter{isPrintingTimeValue: isPrintingTimeValue, fieldNames: fieldNames}
	}
}

type logHandler func(result *logResult)

func (f *RhobsFetcher) queryLogs(ctx context.Context, lokiExpr string, startTime, endTime time.Time, logsCount int, isGoingForward bool, logHandler logHandler) error {
	client, err := f.getClient()
	if err != nil {
		return err
	}

	log.Infoln("RHOBS cell:", f.RhobsCell)
	log.Infoln("Loki query:", lokiExpr)
	log.Infoln("Start time:", startTime.Round(0))
	log.Infoln("End time  :", endTime.Round(0))

	startTimeStamp := startTime.UnixNano()
	endTimeStamp := endTime.UnixNano()

	var logsDir string
	var cmpTimeStamps func(int64, int64) bool

	if isGoingForward {
		logsDir = "forward"
		cmpTimeStamps = func(ts1 int64, ts2 int64) bool {
			return ts1 < ts2
		}
	} else {
		logsDir = "backward"
		cmpTimeStamps = func(ts1 int64, ts2 int64) bool {
			return ts1 > ts2
		}
	}

	for logsCount != 0 {
		lokiQuery := rhobsparameters.LogqlQuery(lokiExpr)

		startTimeStr := strconv.FormatInt(startTimeStamp, 10)
		endTimeStr := strconv.FormatInt(endTimeStamp, 10)

		limit := float32(500)
		if 0 < logsCount && logsCount < 500 {
			limit = float32(logsCount)
		}

		queryParams := &rhobsclient.GetLogRangeQueryParams{
			Query:     &lokiQuery,
			Start:     (*rhobsparameters.StartTS)(&startTimeStr),
			End:       (*rhobsparameters.EndTS)(&endTimeStr),
			Direction: &logsDir,
			Limit:     (*rhobsparameters.Limit)(&limit),
		}

		response, err := client.GetLogRangeQueryWithResponse(ctx, "hcp", queryParams)

		if err != nil {
			return fmt.Errorf("failed to send request to RHOBS: %v", err)
		}
		if response.HTTPResponse.StatusCode != http.StatusOK {
			return fmt.Errorf("RHOBS query failed with status code: %d - body: %s", response.HTTPResponse.StatusCode, string(response.Body))
		}

		var formattedResponse getLogsResponse
		err = json.Unmarshal(response.Body, &formattedResponse)
		if err != nil {
			return fmt.Errorf("failed to unmarshal response from RHOBS: %v", err)
		}

		if formattedResponse.Status != "success" {
			return fmt.Errorf("RHOBS query failed with status: %s", formattedResponse.Status)
		}

		if len(formattedResponse.Data.Results) == 0 {
			break
		}

		{
			var flattenedResults []*logResult

			for _, result := range formattedResponse.Data.Results {
				for valIdx := range result.Values {
					flattenedResults = append(flattenedResults, &logResult{
						Stream: result.Stream,
						Values: []*[]string{result.Values[valIdx]},
					})
				}
			}

			sort.Slice(flattenedResults, func(i, j int) bool {
				return cmpTimeStamps(flattenedResults[i].getTimeStamp(), flattenedResults[j].getTimeStamp())
			})

			edgeTimeStamp := flattenedResults[len(flattenedResults)-1].getTimeStamp()

			if flattenedResults[0].getTimeStamp() == edgeTimeStamp {
				// All logs have the same timestamp, we need to move the time window to some uncharted time to avoid getting stuck
				// We may skip some logs with this approach, but there's no way to get all of them anyway
				if isGoingForward {
					startTimeStamp = edgeTimeStamp + 1
				} else {
					endTimeStamp = edgeTimeStamp - 1
				}
				edgeTimeStamp = 0
			} else {
				if isGoingForward {
					startTimeStamp = edgeTimeStamp
				} else {
					endTimeStamp = edgeTimeStamp
				}
			}

			for _, result := range flattenedResults {
				ts := result.getTimeStamp()

				if ts != edgeTimeStamp {
					logHandler(result)
					logsCount--
					if logsCount == 0 {
						break
					}
				}
			}
		}
	}

	return nil
}

func (f *RhobsFetcher) PrintLogs(ctx context.Context, lokiExpr string, startTime, endTime time.Time, logsCount int, isGoingForward bool, format LogsFormat, isPrintingTimeValue bool, fieldNames []string) error {
	logsPrinter := createLogsPrinter(format, isPrintingTimeValue, fieldNames)
	logsPrinter.PrintHeader()
	defer logsPrinter.PrintTrailer()

	err := f.queryLogs(ctx, lokiExpr, startTime, endTime, logsCount, isGoingForward, func(result *logResult) {
		logsPrinter.PrintResult(result)
	})
	if err != nil {
		return err
	}

	return nil
}

type streamLogsContext struct {
	isLooping bool
	webSocket *websocket.Conn
}

func (f *RhobsFetcher) StreamLogs(lokiExpr string, format LogsFormat, isPrintingTimeValue bool, fieldNames []string) error {
	startTime := time.Now().Add(-5 * time.Minute)
	tokenProvider, err := f.getTokenProvider()
	if err != nil {
		return err
	}

	log.Infoln("RHOBS cell:", f.RhobsCell)
	log.Infoln("Loki query:", lokiExpr)

	logsPrinter := createLogsPrinter(format, isPrintingTimeValue, fieldNames)
	logsPrinter.PrintHeader()
	defer logsPrinter.PrintTrailer()

	wsUrl, err := url.Parse(f.RhobsCell)
	if err != nil {
		return fmt.Errorf("failed to parse RHOBS cell URL: %v", err)
	}

	wsUrl.Scheme = "wss"
	wsUrl.Path = "/api/logs/v1/hcp/loki/api/v1/tail"

	createWebsocket := func() (*websocket.Conn, error) {
		accessToken, err := tokenProvider.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to get access token: %v", err)
		}

		wsUrl.RawQuery = url.Values{
			"query": {lokiExpr},
			"start": {strconv.FormatInt(startTime.UnixNano(), 10)},
			"limit": {"500"},
		}.Encode()

		header := make(http.Header)
		header.Set("Accept", "application/json")
		header.Set("Content-Type", "application/json")
		header.Set("Authorization", "Bearer "+accessToken)

		wsConn, resp, err := websocket.DefaultDialer.Dial(wsUrl.String(), header)
		if err != nil || wsConn == nil {
			if resp != nil {
				buf, _ := io.ReadAll(resp.Body) // nolint

				return nil, fmt.Errorf("failed to connect to websocket: %s (%v)", string(buf), err)
			} else {
				return nil, fmt.Errorf("failed to connect to websocket: %v", err)
			}
		}

		return wsConn, nil
	}

	closeWebsocket := func(wsConn *websocket.Conn) {
		err := wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			log.Infoln("Failed to send websocket close message:", err)
		}
		err = wsConn.Close()
		if err != nil {
			log.Infoln("Failed to close websocket:", err)
		}
	}

	var mutex sync.Mutex
	context := streamLogsContext{
		isLooping: true,
	}

	getContext := func() streamLogsContext {
		mutex.Lock()
		defer mutex.Unlock()

		return context
	}

	closeWebSocketInContext := func(canContinueLooping bool) {
		var currentWsConn *websocket.Conn

		func() {
			mutex.Lock()
			defer mutex.Unlock()

			if !canContinueLooping {
				context.isLooping = false
			}
			currentWsConn = context.webSocket
			context.webSocket = nil
		}()

		if currentWsConn != nil {
			closeWebsocket(currentWsConn)
		}
	}

	storeNewWebsocketInContext := func(newWsConn *websocket.Conn) {
		func() {
			mutex.Lock()
			defer mutex.Unlock()

			if context.isLooping {
				context.webSocket = newWsConn
				newWsConn = nil
			}
		}()

		if newWsConn != nil {
			closeWebsocket(newWsConn)
		}
	}

	go func() { // Break below loop when the user interrupts the command (e.g. by pressing Ctrl+C)
		stopChan := make(chan os.Signal, 1)
		signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
		<-stopChan
		signal.Reset(syscall.SIGTERM)

		closeWebSocketInContext(false)
	}()

	for getContext().isLooping {
		err := backoff.Retry(func() error {
			currentContext := getContext()
			if !currentContext.isLooping || currentContext.webSocket != nil {
				return nil
			}

			newWsConn, err := createWebsocket()
			if err != nil {
				return err
			}
			storeNewWebsocketInContext(newWsConn)

			return nil
		}, backoff.NewExponentialBackOff(
			backoff.WithInitialInterval(500*time.Millisecond),
			backoff.WithMaxInterval(5*time.Second),
			backoff.WithMaxElapsedTime(30*time.Second)))
		if err != nil {
			return fmt.Errorf("failed to establish websocket connection after retrying for 30s: %v", err)
		}

		currentWebSocket := getContext().webSocket
		if currentWebSocket == nil {
			break
		}

		var formattedResponse streamLogsResponse

		err = currentWebSocket.ReadJSON(&formattedResponse)
		if err != nil {
			if !getContext().isLooping {
				break
			}

			if websocket.IsCloseError(err, websocket.CloseAbnormalClosure) {
				log.Warnln("Unexpected websocket close:", err)
				closeWebSocketInContext(true)
				startTime = time.Now()
				log.Infoln("Connecting again...")
				continue
			}

			closeWebSocketInContext(false)
			logsPrinter.PrintTrailer()

			return fmt.Errorf("failed to read JSON from websocket: %v", err)
		}

		for _, result := range formattedResponse.Streams {
			logsPrinter.PrintResult(result)
		}
	}

	return nil
}
