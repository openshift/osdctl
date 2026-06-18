package rhobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	ocmutils "github.com/openshift/osdctl/pkg/utils"

	rhobsclient "github.com/observatorium/api/client"
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

func printAsJson(data interface{}) {
	content, err := json.MarshalIndent(data, "", "  ")
	if err == nil {
		fmt.Println(string(content))
	} else {
		log.Warnln("Failed to marshal json:", err)
	}
}

func printResultsAsJson[result any](results *[]*jsonInterceptor[result]) {
	printAsJson(results)
}
