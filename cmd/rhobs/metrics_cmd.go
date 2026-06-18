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
	"strconv"
	"strings"
	"time"

	rhobsclient "github.com/observatorium/api/client"
	rhobsparameters "github.com/observatorium/api/client/parameters"
	"github.com/pkg/browser"
	log "github.com/sirupsen/logrus"
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
		Args: cobra.MaximumNArgs(1),
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

			rhobsFetcher, err := CreateRhobsFetcher(cmd.Context(), commonOptions.clusterId, RhobsFetchForMetrics, commonOptions.hiveOcmUrl)
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

	cmd.Flags().StringVarP(&outputFormatStr, "output", "o", string(MetricsFormatTable), `Format of the output - allowed values: "table", "csv" or "json" - `+
		`"json" prints raw API data and as such is forward compatible - exclusive with --url`)
	cmd.MarkFlagsMutuallyExclusive("output", "url")
	cmd.Flags().BoolVarP(&isPrintingClusterResultsOnly, "filter", "f", false, "Only keep the results matching the given cluster - "+
		"only effective if some of those results have a _id, _mc_id or mc_name label - exclusive with --url")
	cmd.MarkFlagsMutuallyExclusive("filter", "url")

	return cmd
}

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

func (f *RhobsFetcher) getMetricsGrafanaDataSource() (string, error) {
	baseDataSource, err := f.getBaseGrafanaDataSource()
	if err != nil {
		return "", err
	}

	return baseDataSource + "metrics", nil
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

type getMetricsResponse[metricResult instantMetricResult | rangeMetricResult] struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string                           `json:"resultType"`
		Results    []*jsonInterceptor[metricResult] `json:"result"`
	} `json:"data"`
}

type metricTimestampFormatter func(d *metricData) string

type metricsTableColumn struct {
	name  string
	width int
}

func getMetricsTableColumns(results *[]*jsonInterceptor[instantMetricResult], tsFormatter metricTimestampFormatter) *[]metricsTableColumn {
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
			labelNameToColumn[labelName].width = max(labelNameToColumn[labelName].width, len(labelValue))
		}
	}

	columns := []metricsTableColumn{timeColumn, valueColumn}

	sort.Strings(labelNames)
	for _, labelName := range labelNames {
		columns = append(columns, *labelNameToColumn[labelName])
	}

	return &columns
}

type instantMetricsPrinter func(*[]*jsonInterceptor[instantMetricResult])

func printMetricsAsTable(results *[]*jsonInterceptor[instantMetricResult]) {
	columns := getMetricsTableColumns(results, func(d *metricData) string { return d.getHumanReadableTime() })
	separatorLine := "+"
	for _, column := range *columns {
		separatorLine += strings.Repeat("-", column.width+2) + "+"
	}

	fmt.Println(separatorLine)

	// Header
	fmt.Print("|")
	for _, column := range *columns {
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
		fmt.Printf("| %*s | %*s |", (*columns)[0].width, time, (*columns)[1].width, value)

		for _, column := range (*columns)[2:] {
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
	for _, column := range *columns {
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
		for _, column := range (*columns)[2:] {
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

func createInstantMetricsPrinter(format MetricsFormat) instantMetricsPrinter {
	switch format {
	case MetricsFormatCsv:
		return printMetricsAsCsv
	case MetricsFormatJson:
		return printResultsAsJson[instantMetricResult]
	default:
		return printMetricsAsTable
	}
}

type resultWithLabel interface {
	instantMetricResult | rangeMetricResult | alertResult

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

func filterMetricsResults[metricResult resultWithLabel](fetcher *RhobsFetcher, results *[]*jsonInterceptor[metricResult], isPrintingClusterResultsOnly bool) *[]*jsonInterceptor[metricResult] {
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
		printResultsAsJson[rangeMetricResult](results)
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
