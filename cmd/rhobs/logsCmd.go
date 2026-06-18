package rhobs

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/browser"
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
