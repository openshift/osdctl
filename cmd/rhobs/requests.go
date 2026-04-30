package rhobs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	ocmutils "github.com/openshift/osdctl/pkg/utils"

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

type RhobsFetchUsage string

const (
	RhobsFetchForMetrics RhobsFetchUsage = "metrics"
	RhobsFetchForLogs    RhobsFetchUsage = "logs"
)

type RhobsFetcher struct {
	clusterId           string
	clusterExternalId   string
	clusterName         string
	isManagementCluster bool
	ocmEnvName          string
	RhobsCell           string
}

func getRhobsCellFromConfigMap(clusterId string, ocmConn *sdk.Connection, configMapNamespace, configMapName, configMapAnnotationKey string) (string, error) {
	client, err := k8s.NewWithConn(clusterId, client.Options{}, ocmConn)
	if err != nil {
		return "", fmt.Errorf("failed to create kube client for cluster '%s': %v", clusterId, err)
	}

	var configMap corev1.ConfigMap

	err = client.Get(context.TODO(), types.NamespacedName{Namespace: configMapNamespace, Name: configMapName}, &configMap)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve config map named '%s' in namespace '%s' for cluster '%s': %v", configMapName, configMapNamespace, clusterId, err)
	}

	rhobsCell, ok := configMap.Annotations[configMapAnnotationKey]
	if !ok {
		return "", fmt.Errorf("config map named '%s' in namespace '%s' for cluster '%s' does not have the following annotation key: %s", configMapName, configMapNamespace, clusterId, configMapAnnotationKey)
	}

	return rhobsCell, nil
}

func getMetricsRhobsCellFromConfigMap(clusterId string, ocmConn *sdk.Connection) (string, error) {
	return getRhobsCellFromConfigMap(clusterId, ocmConn, rhobsCellMetricsCmNs, rhobsCellMetricsCmName, rhobsCellCmAnnotation)
}

func getLogsRhobsCellFromConfigMap(clusterId string, ocmConn *sdk.Connection) (string, error) {
	return getRhobsCellFromConfigMap(clusterId, ocmConn, rhobsCellLogsCmNs, rhobsCellLogsCmName, rhobsCellCmAnnotation)
}

func getRhobsCellFromUsage(clusterId string, ocmConn *sdk.Connection, usage RhobsFetchUsage) (string, error) {
	switch usage {
	case RhobsFetchForMetrics:
		return getMetricsRhobsCellFromConfigMap(clusterId, ocmConn)
	case RhobsFetchForLogs:
		return getLogsRhobsCellFromConfigMap(clusterId, ocmConn)
	default:
		return "", fmt.Errorf("unsupported RhobsFetchUsage: %s", usage)
	}
}

func getOtherUsage(usage RhobsFetchUsage) RhobsFetchUsage {
	if usage == RhobsFetchForMetrics {
		return RhobsFetchForLogs
	} else {
		return RhobsFetchForMetrics
	}
}

func getRhobsCellFromHiveClusterDeployment(clusterId string, ocmConn *sdk.Connection, hiveOcmUrl string) (string, error) {
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

	err = hiveClient.List(context.TODO(), &clusterDeployments, &client.ListOptions{LabelSelector: clusterSelector})
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

func getRhobsCell(clusterId string, ocmConn *sdk.Connection, usage RhobsFetchUsage, hiveOcmUrl string) (string, error) {
	rhobsCell, err := getRhobsCellFromUsage(clusterId, ocmConn, usage)
	if err == nil {
		return rhobsCell, nil
	}
	log.Warnf("Failed to get RHOBS cell from %s config map for cluster '%s': %v\n", usage, clusterId, err)
	log.Infoln("Trying to get the RHOBS cell from the hive cluster deployment instead...")

	rhobsCell, err = getRhobsCellFromHiveClusterDeployment(clusterId, ocmConn, hiveOcmUrl)
	if err == nil {
		return rhobsCell, nil
	}
	log.Warnf("Failed to get RHOBS cell from hive cluster deployment for cluster '%s': %v\n", clusterId, err)
	otherUsage := getOtherUsage(usage)
	log.Infof("Trying to get the RHOBS cell from the %s config map instead...", otherUsage)

	rhobsCell, err = getRhobsCellFromUsage(clusterId, ocmConn, otherUsage)
	if err == nil {
		return rhobsCell, nil
	}

	return "", fmt.Errorf("failed to get RHOBS cell for cluster '%s' despite trying all methods", clusterId)
}

func CreateRhobsFetcher(clusterKey string, rhobsFetchUse RhobsFetchUsage, hiveOcmUrl string) (*RhobsFetcher, error) {
	ocmConn, err := ocmutils.CreateConnection()
	if err != nil {
		return nil, err
	}
	defer ocmConn.Close()

	cluster, err := ocmutils.GetCluster(ocmConn, clusterKey)
	if err != nil {
		return nil, err
	}

	var monitoredClusterId string
	isHcp := cluster.Hypershift().Enabled()

	if isHcp {
		if rhobsFetchUse == RhobsFetchForLogs {
			return nil, fmt.Errorf("cluster '%s' is a HCP cluster - try with its parent MC cluster", cluster.ID())
		}
		managementCluster, err := ocmutils.GetManagementCluster(cluster.ID())
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve management cluster for cluster '%s': %v", cluster.ID(), err)
		}
		monitoredClusterId = managementCluster.ID()
		log.Infof("Cluster %s is managed by MC cluster %s - using the MC cluster to retrieve the RHOBS cell for logs\n", cluster.ID(), monitoredClusterId)
	} else {
		monitoredClusterId = cluster.ID()
	}

	rhobsCell, err := getRhobsCell(monitoredClusterId, ocmConn, rhobsFetchUse, hiveOcmUrl)
	if err != nil {
		return nil, err
	}

	// We have to overwrite the fact that backplane just mangled our configuration.
	// TODO: Do not use the global configuration instead (https://issues.redhat.com/browse/OSD-19773)
	err = osdctlConfig.EnsureConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to reload osdctl config: %v", err)
	}

	return &RhobsFetcher{
		clusterId:           cluster.ID(),
		clusterExternalId:   cluster.ExternalID(),
		clusterName:         cluster.Name(),
		isManagementCluster: !isHcp,
		ocmEnvName:          ocmutils.GetCurrentOCMEnv(ocmConn),
		RhobsCell:           rhobsCell,
	}, nil
}

func (q *RhobsFetcher) getClient() (*rhobsclient.ClientWithResponses, error) {
	tokenProvider, err := ocmutils.GetScopedTokenProvider(authUrl, fmt.Sprintf(rhobsVaultPathKeyTemplate, q.ocmEnvName), "profile")
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %v", err)
	}

	rhobsClient, err := rhobsclient.NewClientWithResponses(q.RhobsCell, func(rhobsClient *rhobsclient.Client) error {
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

type getInstantMetricsResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string                `json:"resultType"`
		Results    []instantMetricResult `json:"result"`
	} `json:"data"`
}

type instantMetricResult struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"`
}

type tableColumn struct {
	name  string
	width int
}

func getTableColumns(results *[]instantMetricResult) []tableColumn {
	timeColumn := tableColumn{name: "TIME", width: len("TIME")}
	valueColumn := tableColumn{name: "VALUE", width: len("VALUE")}
	labelNameToColumn := make(map[string]*tableColumn)
	labelNames := []string{} // to maintain order of label columns

	for _, result := range *results {
		if len(result.Value) < 2 {
			continue
		}
		time := fmt.Sprintf("%.3f", result.Value[0])
		if len(time) > timeColumn.width {
			timeColumn.width = len(time)
		}
		value := fmt.Sprintf("%s", result.Value[1])
		if len(value) > valueColumn.width {
			valueColumn.width = len(value)
		}

		for labelName, labelValue := range result.Metric {
			if _, exists := labelNameToColumn[labelName]; !exists {
				labelNameToColumn[labelName] = &tableColumn{name: labelName, width: len(labelName)}
				labelNames = append(labelNames, labelName)
			}
			if len(labelValue) > labelNameToColumn[labelName].width {
				labelNameToColumn[labelName].width = len(labelValue)
			}
		}
	}

	columns := []tableColumn{timeColumn, valueColumn}

	sort.Strings(labelNames)
	for _, labelName := range labelNames {
		columns = append(columns, *labelNameToColumn[labelName])
	}

	return columns
}

type metricsPrinter func(*[]instantMetricResult)

func printMetricsAsTable(results *[]instantMetricResult) {
	columns := getTableColumns(results)
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
		if len(result.Value) < 2 {
			continue
		}
		time := fmt.Sprintf("%.3f", result.Value[0])
		value := fmt.Sprintf("%s", result.Value[1])
		fmt.Printf("| %*s | %*s |", columns[0].width, time, columns[1].width, value)

		for _, column := range columns[2:] {
			labelValue := result.Metric[column.name]
			fmt.Printf(" %*s |", column.width, labelValue)
		}
		fmt.Println()
	}
	fmt.Println(separatorLine)
}

func printMetricsAsCsv(results *[]instantMetricResult) {
	columns := getTableColumns(results)

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
		if len(result.Value) < 2 {
			continue
		}
		row := []string{fmt.Sprintf("%.3f", result.Value[0]), fmt.Sprintf("%s", result.Value[1])}
		for _, column := range columns[2:] {
			row = append(row, result.Metric[column.name])
		}
		err := writer.Write(row)
		if err != nil {
			log.Warnln("Failed to write CSV row:", err)
			return
		}
	}

	writer.Flush()
}

func printMetricsAsJson(results *[]instantMetricResult) {
	metricsBytes, err := json.MarshalIndent(results, "", "  ")
	if err == nil {
		fmt.Println(string(metricsBytes))
	} else {
		log.Warnln("Failed to marshal metrics data:", err)
	}
}

func createMetricsPrinter(format MetricsFormat) metricsPrinter {
	switch format {
	case MetricsFormatCsv:
		return printMetricsAsCsv
	case MetricsFormatJson:
		return printMetricsAsJson
	default:
		return printMetricsAsTable
	}
}

func (q *RhobsFetcher) PrintMetrics(promExpr string, format MetricsFormat, isPrintingClusterResultsOnly bool) error {
	client, err := q.getClient()
	if err != nil {
		return err
	}

	log.Infoln("RHOBS cell:", q.RhobsCell)

	promQuery := rhobsparameters.PromqlQuery(promExpr)
	queryParams := &rhobsclient.GetInstantQueryParams{Query: &promQuery}

	response, err := client.GetInstantQueryWithResponse(context.TODO(), "hcp", queryParams)

	if err != nil {
		return fmt.Errorf("failed to send request to RHOBS: %v", err)
	}
	if response.HTTPResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("RHOBS query failed with status code: %d - body: %s", response.HTTPResponse.StatusCode, string(response.Body))
	}

	var formattedResponse getInstantMetricsResponse
	err = json.Unmarshal(response.Body, &formattedResponse)
	if err != nil {
		return fmt.Errorf("failed to unmarshal response from RHOBS: %v", err)
	}

	if formattedResponse.Status != "success" {
		return fmt.Errorf("RHOBS query failed with status: %s", formattedResponse.Status)
	}

	areSomeResultsForOtherClusters := false // If true, this means areFilteredResultsValid is also true
	filteredResults := []instantMetricResult{}
	areFilteredResultsValid := len(formattedResponse.Data.Results) == 0

	for _, result := range formattedResponse.Data.Results {
		if q.isManagementCluster {
			mcId, hasMcId := result.Metric["_mc_id"]
			mcName, hasMcName := result.Metric["mc_name"]
			if !hasMcId && !hasMcName {
				continue
			}
			areFilteredResultsValid = true
			if mcId != q.clusterId && mcName != q.clusterName {
				areSomeResultsForOtherClusters = mcId != "" || mcName != ""
				continue
			}
		} else {
			extId, hasExtId := result.Metric["_id"]
			if !hasExtId {
				continue
			}
			areFilteredResultsValid = true
			if extId != q.clusterExternalId {
				areSomeResultsForOtherClusters = extId != ""
				continue
			}
		}
		filteredResults = append(filteredResults, result)
	}

	var resultsToPrint *[]instantMetricResult

	if isPrintingClusterResultsOnly && areFilteredResultsValid {
		resultsToPrint = &filteredResults
	} else {
		if isPrintingClusterResultsOnly {
			log.Warnln("Results returned by RHOBS cannot be matched against the given cluster. Working as if --filter option was not set.")
		}
		resultsToPrint = &formattedResponse.Data.Results
		if !isPrintingClusterResultsOnly && areSomeResultsForOtherClusters {
			log.Warnln("Printing ALL RHOBS cell results even the ones not matching the given cluster. " +
				"You could have used the --filter option to only print the results matching the given cluster.")
		} else {
			log.Infoln("Printing ALL RHOBS cell results")
		}
	}

	createMetricsPrinter(format)(resultsToPrint)

	return nil
}

type getLogsResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string       `json:"resultType"`
		Results    []*logResult `json:"result"`
	} `json:"data"`
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

func (q *RhobsFetcher) PrintLogs(lokiExpr string, startTime, endTime time.Time, logsCount int, isGoingForward bool, format LogsFormat, isPrintingTimeValue bool, fieldNames []string) error {
	client, err := q.getClient()
	if err != nil {
		return err
	}

	log.Infoln("RHOBS cell:", q.RhobsCell)
	log.Infoln("Loki query:", lokiExpr)
	log.Infoln("Start time:", startTime.Round(0))
	log.Infoln("End time  :", endTime.Round(0))

	logsPrinter := createLogsPrinter(format, isPrintingTimeValue, fieldNames)
	logsPrinter.PrintHeader()

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

		response, err := client.GetLogRangeQueryWithResponse(context.TODO(), "hcp", queryParams)

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
					logsPrinter.PrintResult(result)
					logsCount--
					if logsCount == 0 {
						break
					}
				}
			}
		}
	}
	logsPrinter.PrintTrailer()

	return nil
}
