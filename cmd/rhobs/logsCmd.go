package rhobs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
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

	"github.com/cenkalti/backoff/v4"
	"github.com/gorilla/websocket"
	rhobsclient "github.com/observatorium/api/client"
	rhobsparameters "github.com/observatorium/api/client/parameters"
	"github.com/pkg/browser"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

var allowedLogLevels = []string{"default", "trace", "info", "warn", "error"}

func newCmdLogs() *cobra.Command {
	var isComputingGrafanaUrl bool
	var isOpeningGrafanaUrl bool
	var lokiExpr string
	var namespace string
	var labelSelectorStr string
	var containerName string
	var isIncludingEvents bool
	var containedStrings []string
	var notContainedStrings []string
	var containedRegexps []string
	var notContainedRegexps []string
	var logLevels []string
	var startTime time.Time
	var endTime time.Time
	var duration time.Duration
	var direction string
	var isFollowing bool
	var logsCount int
	var isNotLimitingLogsCount bool
	var outputFormatStr string
	var isPrintingTimestamp bool
	var printedFields []string

	cmd := &cobra.Command{
		Use:   "logs [pod]",
		Short: "Fetch logs from RHOBS for a given cluster",
		Long: "Fetch logs from RHOBS for a given cluster. " +
			"The cluster can be a management cluster (MC) or whatever cluster sending logs to RHOBS; " +
			"the command works as if the management cluster ID was passed if given a hosted cluster (HCP) ID. " +
			"By default, logs from all the pods in the given namespace are returned but it is possible to specify " +
			"a single pod as an argument or filter pods using their labels. Logs themselves can be also filtered " +
			"to only keep the ones containing a given regexp (--contain-regex option) or a given log level (--level option).",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isOpeningGrafanaUrl && !isComputingGrafanaUrl {
				return fmt.Errorf("--browser can only be set if --url is set")
			}

			if cmd.Flags().Changed("query") {
				if len(args) > 0 {
					return errors.New("pod argument cannot be used with --query flag")
				}
			} else {
				lokiExpr = fmt.Sprintf(`{k8s_namespace_name="%s"}`, namespace)

				specialCharsRegex := regexp.MustCompile(`["\\]`)
				esc := func(str string) string {
					return specialCharsRegex.ReplaceAllString(str, "\\$0")
				}

				filterMessage := func(lokiOp string, values []string) {
					for _, value := range values {
						lokiExpr += fmt.Sprintf(` %s "%s"`, lokiOp, esc(value))
					}
				}
				filterMessage("|=", containedStrings)
				filterMessage("!=", notContainedStrings)
				filterMessage("|~", containedRegexps)
				filterMessage("!~", notContainedRegexps)

				if len(args) == 1 {
					lokiExpr += fmt.Sprintf(` | k8s_pod_name="%s"`, args[0])
				}

				if cmd.Flags().Changed("selector") {
					selector, err := labels.Parse(labelSelectorStr)
					if err != nil {
						return fmt.Errorf("invalid label selector: %v", err)
					}
					requirements, selectable := selector.Requirements()
					if selectable {
						nonAlphanumericRegex := regexp.MustCompile("[^a-zA-Z0-9_]")

						for _, req := range requirements {
							op := req.Operator()
							lokiOp := "="
							switch op {
							case selection.NotEquals:
								lokiOp = "!="
							case selection.Exists, selection.In:
								lokiOp = "=~"
							case selection.DoesNotExist, selection.NotIn:
								lokiOp = "!~"
							}
							normKey := "k8s_pod_label_" + nonAlphanumericRegex.ReplaceAllString(req.Key(), "_")
							values := req.ValuesUnsorted()
							switch op {
							case selection.Equals, selection.DoubleEquals, selection.NotEquals:
								if len(values) != 1 {
									return fmt.Errorf("internal error: label selector operator '%s' is expected to have only one value in '%s'", op, labelSelectorStr)
								}
								lokiExpr += fmt.Sprintf(` | %s %s "%s"`, normKey, lokiOp, esc(values[0]))
							case selection.Exists, selection.DoesNotExist:
								if len(values) != 0 {
									return fmt.Errorf("internal error: label selector operator '%s' is not expected to have any value in '%s'", op, labelSelectorStr)
								}
								lokiExpr += fmt.Sprintf(` | %s %s ".*"`, normKey, lokiOp)
							case selection.In, selection.NotIn:
								lokiExpr += fmt.Sprintf(` | %s %s "`, normKey, lokiOp)
								for k, val := range values {
									if k > 0 {
										lokiExpr += "|"
									}
									lokiExpr += fmt.Sprintf("(%s)", esc(regexp.QuoteMeta(val)))
								}
								lokiExpr += `"`
							default:
								return fmt.Errorf("internal error: label selector operator '%s' is not implemented", req.Operator())
							}
						}
					}
				}

				if cmd.Flags().Changed("container") {
					lokiExpr += fmt.Sprintf(` | k8s_container_name="%s"`, containerName)
				}

				if !isIncludingEvents {
					lokiExpr += ` | json json_kind="kind" | json_kind != "Event"`
				}

				if len(logLevels) > 0 {
					isLevelSelected := map[string]bool{}
					for _, level := range allowedLogLevels {
						isLevelSelected[level] = false
					}
					lokiExpr += ` | level =~ "(`
					for k, level := range logLevels {
						isSelected, isAllowed := isLevelSelected[level]
						if !isAllowed {
							return fmt.Errorf("invalid log level '%s', allowed values are: %s", level, strings.Join(allowedLogLevels, ", "))
						}
						if isSelected {
							return fmt.Errorf("log level '%s' is specified multiple times, each log level should be specified only once", level)
						}
						isLevelSelected[level] = true
						if k > 0 {
							lokiExpr += "|"
						}
						lokiExpr += level
					}
					lokiExpr += `)"`
				}
			}

			nowTime := time.Now()
			defaultStartTime := nowTime.Add(-5 * time.Minute)

			if cmd.Flags().Changed("since") {
				if duration <= 0 {
					return fmt.Errorf("--since must be greater than 0")
				}
				startTime = nowTime.Add(-duration)
			} else if !cmd.Flags().Changed("start-time") && !cmd.Flags().Changed("since-time") {
				startTime = defaultStartTime
			}
			if !cmd.Flags().Changed("end-time") {
				endTime = nowTime
			}
			if startTime.After(endTime) {
				return fmt.Errorf("value passed to --start-time must be before the value passed to --end-time")
			}

			isGoingForward := false
			if cmd.Flags().Changed("direction") {
				if direction != "forward" && direction != "backward" {
					return fmt.Errorf("invalid value for --direction flag: %s, it must be either \"forward\" or \"backward\"", direction)
				}
				isGoingForward = direction == "forward"
			}

			if cmd.Flags().Changed("limit") {
				if logsCount < 1 || 100000 < logsCount {
					return fmt.Errorf("invalid value for --limit flag: %d, it must be between 1 and 100000", logsCount)
				}
			} else {
				logsCount = 10000
				if isNotLimitingLogsCount {
					logsCount = -1
				}
			}

			outputFormat, err := GetLogsFormatFromString(outputFormatStr)
			if err != nil {
				return err
			}

			if outputFormat == LogsFormatJson {
				if cmd.Flags().Changed("ts") {
					return fmt.Errorf("--ts flag cannot be used with json output format")
				}
				if cmd.Flags().Changed("field") {
					return fmt.Errorf("--field flag cannot be used with json output format")
				}
			}

			cmd.SilenceUsage = true

			rhobsFetcher, err := CreateRhobsFetcher(cmd.Context(), commonOptions.clusterId, RhobsFetchForLogs, commonOptions.hiveOcmUrl)
			if err != nil {
				return err
			}

			if !cmd.Flags().Changed("query") {
				lokiExpr += fmt.Sprintf(` | openshift_cluster_id = "%s"`, rhobsFetcher.clusterExternalId)
			}

			if isComputingGrafanaUrl {
				grafanaUrl, err := rhobsFetcher.GetGrafanaLogsUrl(lokiExpr, startTime, endTime, isGoingForward)
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
				if isFollowing {
					err = rhobsFetcher.StreamLogs(lokiExpr, outputFormat, isPrintingTimestamp, printedFields)
				} else {
					err = rhobsFetcher.PrintLogs(cmd.Context(), lokiExpr, startTime, endTime, logsCount, isGoingForward, outputFormat, isPrintingTimestamp, printedFields)
				}
				if err != nil {
					return fmt.Errorf("failed to print logs: %v", err)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&isComputingGrafanaUrl, "url", "u", false, "Only compute and print the grafana URL")
	cmd.Flags().BoolVarP(&isOpeningGrafanaUrl, "browser", "b", false, "Open in the default browser the URL computed with the --url option - only applicable if --url is set")

	cmd.Flags().StringVarP(&lokiExpr, "query", "q", "", "LogQL expression - exclusive with many other flags")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Name of the namespace")
	cmd.Flags().StringVarP(&labelSelectorStr, "selector", "l", "", "Label selector for filtering pods - exclusive with the pod argument")
	cmd.Flags().StringVarP(&containerName, "container", "c", "", "Name of the container - print all containers logs if not specified")
	cmd.Flags().BoolVar(&isIncludingEvents, "include-events", false, "Include events in the logs output - may add significant noise, use with caution")
	cmd.Flags().StringArrayVar(&containedStrings, "contain", []string{}, "Text the log message must contain - flag can be repeated")
	cmd.Flags().StringArrayVar(&notContainedStrings, "not-contain", []string{}, "Text the log message must not contain - flag can be repeated")
	cmd.Flags().StringArrayVar(&containedRegexps, "contain-regex", []string{}, "Regular expression the log message must contain - flag can be repeated")
	cmd.Flags().StringArrayVar(&notContainedRegexps, "not-contain-regex", []string{}, "Regular expression the log message must not contain - flag can be repeated")
	cmd.Flags().StringSliceVar(&logLevels, "level", []string{}, fmt.Sprintf("Log level to retain - allowed values: %s", `"`+strings.Join(allowedLogLevels, `", "`)+`"`)+
		" - flag can be repeated / values can also be aggregated with one flag using the comma as separator")
	cmd.MarkFlagsMutuallyExclusive("query", "contain")
	cmd.MarkFlagsMutuallyExclusive("query", "not-contain")
	cmd.MarkFlagsMutuallyExclusive("query", "contain-regex")
	cmd.MarkFlagsMutuallyExclusive("query", "not-contain-regex")
	cmd.MarkFlagsMutuallyExclusive("query", "level")
	cmd.MarkFlagsMutuallyExclusive("query", "namespace")
	cmd.MarkFlagsMutuallyExclusive("query", "selector")
	cmd.MarkFlagsMutuallyExclusive("query", "container")
	cmd.MarkFlagsMutuallyExclusive("query", "include-events")

	cmd.Flags().TimeVar(&startTime, "start-time", time.Time{}, []string{time.RFC3339}, "Start time for the logs - alternate alias: --since-time (default to 5 minutes ago)")
	cmd.Flags().TimeVar(&endTime, "end-time", time.Time{}, []string{time.RFC3339}, "End time for the logs (default to now)")
	cmd.Flags().TimeVar(&startTime, "since-time", time.Time{}, []string{time.RFC3339}, "Same as --start-time")
	_ = cmd.Flags().MarkHidden("since-time")
	cmd.Flags().DurationVar(&duration, "since", 0, "Only return logs newer than a relative duration (e.g. 1h, 30m) - exclusive with --start-time & --end-time")
	cmd.MarkFlagsMutuallyExclusive("start-time", "since")
	cmd.MarkFlagsMutuallyExclusive("since-time", "since")
	cmd.MarkFlagsMutuallyExclusive("end-time", "since")

	cmd.Flags().StringVar(&direction, "direction", "", `Direction of the logs to return - allowed values: "forward" or "backward" - `+
		`"backward" returns the most recent & interesting logs first, while "forward" matches the behavior of "kubectl logs" by returning the oldest logs first `+
		`(default to "backward" unless --follow is set in which case it is forced to "forward")`)

	cmd.Flags().BoolVarP(&isFollowing, "follow", "f", false, "Specify if the logs should be streamed - exclusive with --url, --start-time, --end-time, --since, --direction, --limit and --no-limit flags")
	cmd.MarkFlagsMutuallyExclusive("follow", "start-time")
	cmd.MarkFlagsMutuallyExclusive("follow", "since-time")
	cmd.MarkFlagsMutuallyExclusive("follow", "end-time")
	cmd.MarkFlagsMutuallyExclusive("follow", "since")
	cmd.MarkFlagsMutuallyExclusive("follow", "direction")

	cmd.Flags().IntVar(&logsCount, "limit", 0, "Maximum number of logs to return - allowed range: [1 100000] - exclusive with --no-limit, --url & --follow flags (default to 10000, no limit if --follow is set)")
	cmd.Flags().BoolVar(&isNotLimitingLogsCount, "no-limit", false, "Do not limit the number of logs to return - exclusive with --limit, --url & --follow flags")
	cmd.MarkFlagsMutuallyExclusive("limit", "no-limit", "url", "follow")

	cmd.Flags().StringVarP(&outputFormatStr, "output", "o", string(LogsFormatText), `Format of the output - allowed values: "text", "csv" or "json" - exclusive with --url`)
	cmd.MarkFlagsMutuallyExclusive("output", "url")
	cmd.Flags().BoolVar(&isPrintingTimestamp, "ts", false, `Print metadata timestamps - to be used when log messages do not have a timestamp - not possible with the "json" output format - exclusive with --url`)
	cmd.MarkFlagsMutuallyExclusive("ts", "url")
	cmd.Flags().StringSliceVar(&printedFields, "field", []string{"k8s_pod_name"}, `Fields to print with the log message - not possible with the "json" output format - `+
		`flag can be repeated / values can also be aggregated with one flag using the comma as separator - possible values: "k8s_namespace_name", "k8s_pod_name", "k8s_container_name" - `+
		`use the "json" output format to know about all possible fields - exclusive with --url`)
	cmd.MarkFlagsMutuallyExclusive("field", "url")

	return cmd
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

func (f *RhobsFetcher) getLogsGrafanaDataSource() (string, error) {
	baseDataSource, err := f.getBaseGrafanaDataSource()
	if err != nil {
		return "", err
	}

	return baseDataSource + "logs", nil
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

func (l *logResult) getHumanReadableTime() string {
	return l.getTime().String()
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
		sb.WriteString(result.getHumanReadableTime())
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
		row = append(row, result.getHumanReadableTime())
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
