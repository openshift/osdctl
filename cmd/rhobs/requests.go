package rhobs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/openshift/osdctl/cmd/requester"
	"github.com/openshift/osdctl/pkg/k8s"
	ocmutils "github.com/openshift/osdctl/pkg/utils"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	authUrl                   = "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token"
	rhobsVaultPathKeyTemplate = "rhobs_%s_vault_path"
	clusterIdCdLabel          = "api.openshift.com/id"
	rhobsCellCdLabel          = "ext-hypershift.openshift.io/rhobs-cell"
)

type MetricsFormat string

const (
	MetricsFormatTable MetricsFormat = "table"
	MetricsFormatCsv   MetricsFormat = "csv"
	MetricsFormatJson  MetricsFormat = "json"
)

func GetRhobsCell(clusterKey string) (string, error) {
	connection, err := ocmutils.CreateConnection()
	if err != nil {
		return "", err
	}
	defer connection.Close()

	cluster, err := ocmutils.GetCluster(connection, clusterKey)
	if err != nil {
		return "", err
	}

	if cluster.Hypershift().Enabled() {
		cluster, err = ocmutils.GetManagementCluster(cluster.ID())
		if err != nil {
			return "", fmt.Errorf("failed to retrieve management cluster for cluster '%s': %v", cluster.ID(), err)
		}
	} else {
		isMC, err := ocmutils.IsManagementCluster(cluster.ID())
		if err != nil {
			return "", fmt.Errorf("failed to determine if cluster '%s' is a management cluster: %v", cluster.ID(), err)
		}
		if !isMC {
			return "", fmt.Errorf("cluster '%s' is not a HCP or MC, cannot determine RHOBS cell", cluster.ID())
		}
	}

	hiveCluster, err := ocmutils.GetHiveCluster(cluster.ID())
	if err != nil {
		return "", fmt.Errorf("failed to retrieve hive cluster for cluster '%s': %v", cluster.ID(), err)
	}

	hiveClient, err := k8s.NewWithConn(hiveCluster.ID(), client.Options{}, connection)
	if err != nil {
		return "", fmt.Errorf("failed to create kube client for hive cluster '%s': %v", hiveCluster.ID(), err)
	}

	clusterSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: map[string]string{
		clusterIdCdLabel: cluster.ID(),
	}})
	if err != nil {
		return "", fmt.Errorf("failed to create label selector for cluster '%s': %v", cluster.ID(), err)
	}

	var clusterDeployments hivev1.ClusterDeploymentList

	hiveClient.List(context.TODO(), &clusterDeployments, &client.ListOptions{LabelSelector: clusterSelector})
	if err != nil {
		return "", fmt.Errorf("failed to list cluster deployments for cluster '%s': %v", cluster.ID(), err)
	}

	if len(clusterDeployments.Items) != 1 {
		return "", fmt.Errorf("expected to find exactly 1 cluster deployment for cluster '%s', but found %d", cluster.ID(), len(clusterDeployments.Items))
	}

	clusterDeployment := clusterDeployments.Items[0]
	rhobsCell, ok := clusterDeployment.Labels[rhobsCellCdLabel]

	if !ok {
		return "", fmt.Errorf("cluster deployment for cluster '%s' does not have the expected label", cluster.ID())
	}

	return rhobsCell, nil
}

func getAccessToken(clusterKey string) (string, error) {
	connection, err := ocmutils.CreateConnection()
	if err != nil {
		return "", err
	}
	defer connection.Close()

	envName := ocmutils.GetCurrentOCMEnv(connection)

	return requester.GetScopedAccessToken(authUrl, fmt.Sprintf(rhobsVaultPathKeyTemplate, envName), "profile")
}

type rhobsMetricsResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string        `json:"resultType"`
		Result     []rhobsMetric `json:"result"`
	} `json:"data"`
}

type rhobsMetric struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"`
}

func printMetrics(clusterKey, promExpr string, format MetricsFormat) error {
	accessToken, err := getAccessToken(clusterKey)
	if err != nil {
		return fmt.Errorf("failed to get access token: %v", err)
	}

	rhobsCell, err := GetRhobsCell(clusterKey)
	if err != nil {
		return fmt.Errorf("failed to get RHOBS cell: %v", err)
	}

	queryData := url.Values{
		"query": {promExpr},
	}.Encode()

	requester := requester.Requester{
		Method: http.MethodGet,
		Url:    "https://" + rhobsCell + "/api/metrics/v1/hcp/api/v1/query?" + queryData,
		Headers: map[string]string{
			"Accept":        "application/json",
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + accessToken,
		},
		SuccessCode: http.StatusOK,
	}

	resp, err := requester.Send()
	if err != nil {
		return fmt.Errorf("failed to send request to RHOBS: %v", err)
	}

	var metricsResp rhobsMetricsResponse
	err = json.Unmarshal([]byte(resp), &metricsResp)
	if err != nil {
		return fmt.Errorf("failed to unmarshal response from RHOBS: %v", err)
	}

	if metricsResp.Status != "success" {
		return fmt.Errorf("RHOBS query failed with status: %s", metricsResp.Status)
	}

	switch format {
	case MetricsFormatTable:
		printMetricsAsTable(metricsResp.Data.Result)
	case MetricsFormatCsv:
		printMetricsAsCsv(metricsResp.Data.Result)
	case MetricsFormatJson:
		return printMetricsAsJson(metricsResp.Data.Result)
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}

	return nil
}

type tableColumn struct {
	name  string
	width int
}

func getTableColumns(metrics []rhobsMetric) []tableColumn {
	timeColumn := tableColumn{name: "TIME", width: len("TIME")}
	valueColumn := tableColumn{name: "VALUE", width: len("VALUE")}
	labelNameToColumn := make(map[string]*tableColumn)
	labelNames := []string{} // to maintain order of label columns

	for _, metric := range metrics {
		if len(metric.Value) < 2 {
			continue
		}
		time := fmt.Sprintf("%.3f", metric.Value[0])
		if len(time) > timeColumn.width {
			timeColumn.width = len(time)
		}
		value := fmt.Sprintf("%s", metric.Value[1])
		if len(value) > valueColumn.width {
			valueColumn.width = len(value)
		}

		for labelName, labelValue := range metric.Metric {
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

func printMetricsAsTable(metrics []rhobsMetric) {
	columns := getTableColumns(metrics)
	separatorLine := "+"
	for _, column := range columns {
		separatorLine += strings.Repeat("-", column.width+2) + "+"
	}

	fmt.Println(separatorLine)

	// Header
	fmt.Print("|")
	for _, column := range columns {
		fmt.Print(fmt.Sprintf(" %-*s |", column.width, column.name))
	}
	fmt.Println()
	fmt.Println(separatorLine)

	// Rows

	for _, metric := range metrics {
		if len(metric.Value) < 2 {
			continue
		}
		time := fmt.Sprintf("%.3f", metric.Value[0])
		value := fmt.Sprintf("%s", metric.Value[1])
		fmt.Print(fmt.Sprintf("| %*s | %*s |", columns[0].width, time, columns[1].width, value))

		for _, column := range columns[2:] {
			labelValue := metric.Metric[column.name]
			fmt.Print(fmt.Sprintf(" %*s |", column.width, labelValue))
		}
		fmt.Println()
	}
	fmt.Println(separatorLine)
}

func printMetricsAsCsv(metrics []rhobsMetric) {
	columns := getTableColumns(metrics)

	writer := csv.NewWriter(os.Stdout)

	header := []string{}
	for _, column := range columns {
		header = append(header, column.name)
	}
	writer.Write(header)

	for _, metric := range metrics {
		if len(metric.Value) < 2 {
			continue
		}
		row := []string{fmt.Sprintf("%.3f", metric.Value[0]), fmt.Sprintf("%s", metric.Value[1])}
		for _, column := range columns[2:] {
			row = append(row, metric.Metric[column.name])
		}
		writer.Write(row)
	}

	writer.Flush()
}

func printMetricsAsJson(metrics []rhobsMetric) error {
	metricsBytes, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metrics data: %v", err)
	}

	fmt.Println(string(metricsBytes))

	return nil
}
