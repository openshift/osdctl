package rhobs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"os/user"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	rhobsclient "github.com/observatorium/api/client"
	rhobsmodels "github.com/observatorium/api/client/models"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newCmdAlerts() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alerts",
		Short: "List or silence RHOBS alerts",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(newCmdAlertsGet())
	cmd.AddCommand(newCmdRules())
	cmd.AddCommand(newCmdSilences())

	return cmd
}

func newCmdAlertsGet() *cobra.Command {
	var outputFormatStr string
	var isPrintingClusterResultsOnly bool

	cmd := &cobra.Command{
		Use:   "get",
		Short: "List alerts from RHOBS for a given cluster",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = false

			outputFormat, err := GetAlertsFormatFromString(outputFormatStr)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			rhobsFetcher, err := CreateRhobsFetcher(cmd.Context(), commonOptions.clusterId, RhobsFetchForMetrics, commonOptions.hiveOcmUrl)
			if err != nil {
				return err
			}

			err = rhobsFetcher.PrintAlerts(cmd.Context(), outputFormat, isPrintingClusterResultsOnly)
			if err != nil {
				return fmt.Errorf("failed to print alerts: %v", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFormatStr, "output", "o", string(AlertsFormatText), `Format of the output - allowed values: "text", "csv" or "json"`)
	cmd.Flags().BoolVarP(&isPrintingClusterResultsOnly, "filter", "f", false, "Only keep the results matching the given cluster - "+
		"only effective if some of those results have a _id, _mc_id or mc_name label")

	return cmd
}

func newCmdRules() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prom-rules",
		Short: "The Prometheus rules (alerts & recording rules) defined on the RHOBS cell",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(newCmdRulesGet())

	return cmd
}

func newCmdRulesGet() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "List the Prometheus rules defined on the RHOBS cell",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rhobsFetcher, err := CreateRhobsFetcher(cmd.Context(), commonOptions.clusterId, RhobsFetchForMetrics, commonOptions.hiveOcmUrl)
			if err != nil {
				return err
			}

			body, err := rhobsFetcher.QueryRules(cmd.Context(), "")
			if err != nil {
				return fmt.Errorf("failed to query rules: %v", err)
			}
			printAsJson(body)

			return nil
		},
	}

	return cmd
}

func newCmdSilences() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "silences",
		Short: "The alerts silences defined at RHOBS cell level",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(newCmdSilencesGet())
	cmd.AddCommand(newCmdSilencesCreate())
	cmd.AddCommand(newCmdSilencesDelete())

	return cmd
}

func newCmdSilencesGet() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "List RHOBS cell silences",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rhobsFetcher, err := CreateRhobsFetcher(cmd.Context(), commonOptions.clusterId, RhobsFetchForMetrics, commonOptions.hiveOcmUrl)
			if err != nil {
				return err
			}

			body, err := rhobsFetcher.QuerySilences(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to query silences: %v", err)
			}
			printAsJson(body)

			return nil
		},
	}

	return cmd
}

type alertsSelectorOp struct {
	symbol   string
	isEqual  bool
	isRegexp bool
}

var allAlertsSelectorOps = []alertsSelectorOp{
	{
		symbol:   "==",
		isEqual:  true,
		isRegexp: false,
	},
	{
		symbol:   "!=",
		isEqual:  false,
		isRegexp: false,
	},
	{
		symbol:   "=~",
		isEqual:  true,
		isRegexp: true,
	},
	{
		symbol:   "!~",
		isEqual:  false,
		isRegexp: true,
	},
}

type alertsSelector struct {
	labelName  string
	op         *alertsSelectorOp
	labelValue string
}

func newCmdSilencesCreate() *cobra.Command {
	var startTime, endTime time.Time
	var duration time.Duration
	var author, comment string

	cmd := &cobra.Command{
		Use:   "create selector",
		Short: "Create a silence at RHOBS cell level",
		Long: "Create a silence at RHOBS cell level. " +
			"The mandatory selector argument filters the alerts on which the silence will apply; " +
			"Use ==, !=, =~ and !~ as an operator between a label key and its value; for instance: key1=value1,key2!=value2; " +
			"Use a comma to separate the constraints if more than one or repeat this argument: key1==value1 key2!=value2; " +
			"Special characters (like the ones used in operators) need to be back-slashed if present in a key or, more likely, in a value; " +
			"same applies to the backslash character itself.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = false

			nowTime := time.Now()
			defaultStartTime := nowTime

			if !cmd.Flags().Changed("start-time") {
				startTime = defaultStartTime
			}
			if cmd.Flags().Changed("expire-after") {
				if duration <= 0 {
					return fmt.Errorf("--expire-after must be greater than 0")
				}
				endTime = nowTime.Add(duration)
			}
			if startTime.After(endTime) {
				return fmt.Errorf("value passed to --start-time must be before the value passed to --end-time")
			}

			selectors := []*alertsSelector{}
			specialChars := `=!~,\`
			esc := func(s string) string {
				for _, c := range specialChars {
					s = strings.ReplaceAll(s, `\`+string(c), `\%`+fmt.Sprintf("%x", int(c)))
				}
				return s
			}
			unesc := func(s string) string {
				for _, c := range specialChars {
					s = strings.ReplaceAll(s, `\%`+fmt.Sprintf("%x", int(c)), string(c))
				}
				return s
			}

			for _, arg := range args {
				for _, selectorStr := range strings.Split(esc(arg), ",") {
					var selector *alertsSelector
					for _, op := range allAlertsSelectorOps {
						idx := strings.Index(selectorStr, op.symbol)
						if idx != -1 {
							selector = &alertsSelector{
								labelName:  unesc(selectorStr[:idx]),
								op:         &op,
								labelValue: unesc(selectorStr[idx+len(op.symbol):]),
							}
							break
						}
					}
					if selector == nil {
						return fmt.Errorf("invalid argument / not a valid alert selector: %s", arg)
					}
					selectors = append(selectors, selector)
				}
			}

			if author == "" {
				user, err := user.Current()
				if err == nil {
					author = user.Name
				} else {
					log.Warnln("Failed to determine the current OS user:", err)
					author = "osdctl"
				}
			}

			if comment == "" {
				comment = "created by " + author
			}

			cmd.SilenceUsage = true

			rhobsFetcher, err := CreateRhobsFetcher(cmd.Context(), commonOptions.clusterId, RhobsFetchForMetrics, commonOptions.hiveOcmUrl)
			if err != nil {
				return err
			}

			err = rhobsFetcher.createSilence(cmd.Context(), &selectors, startTime, endTime, author, comment)
			if err != nil {
				return fmt.Errorf("failed to create silence: %v", err)
			}

			return nil
		},
	}
	cmd.Flags().TimeVar(&startTime, "start-time", time.Time{}, []string{time.RFC3339}, "Time at which the silence will start to take effect (defaults to now)")
	cmd.Flags().TimeVar(&endTime, "end-time", time.Time{}, []string{time.RFC3339}, "Time at which the silence will expire - Mandatory unless --expire-after is set")
	cmd.Flags().DurationVar(&duration, "expire-after", 0, "Duration (e.g. 24h, 30m) after which the silence will expire - exclusive with --start-time & --end-time")
	cmd.MarkFlagsMutuallyExclusive("start-time", "expire-after")
	cmd.MarkFlagsMutuallyExclusive("end-time", "expire-after")
	cmd.MarkFlagsOneRequired("end-time", "expire-after")
	cmd.Flags().StringVar(&author, "author", "", "Name of the person creating the silence (default to the OS user name)")
	cmd.Flags().StringVar(&comment, "comment", "", "Some free text giving some context around why the silence is created - "+
		"you can give JIRA or other references there")

	return cmd
}

func newCmdSilencesDelete() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [silence-id]",
		Short: "Expire the given silence from the RHOBS cell",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = false

			silenceId, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("argument is not a UUID")
			}

			cmd.SilenceUsage = true

			rhobsFetcher, err := CreateRhobsFetcher(cmd.Context(), commonOptions.clusterId, RhobsFetchForMetrics, commonOptions.hiveOcmUrl)
			if err != nil {
				return err
			}

			err = rhobsFetcher.DeleteSilence(cmd.Context(), silenceId)
			if err != nil {
				return fmt.Errorf("failed to delete silence: %v", err)
			}

			return nil
		},
	}

	return cmd
}

type AlertsFormat string

const (
	AlertsFormatText AlertsFormat = "text"
	AlertsFormatCsv  AlertsFormat = "csv"
	AlertsFormatJson AlertsFormat = "json"
)

func GetAlertsFormatFromString(formatStr string) (AlertsFormat, error) {
	switch formatStr {
	case string(AlertsFormatText):
		return AlertsFormatText, nil
	case string(AlertsFormatCsv):
		return AlertsFormatCsv, nil
	case string(AlertsFormatJson):
		return AlertsFormatJson, nil
	default:
		return AlertsFormatText, fmt.Errorf("invalid output format: %s", formatStr)
	}
}

type getAlertsResponse = []*jsonInterceptor[alertResult]

type alertResult rhobsmodels.GettableAlert

func (result alertResult) getLabel(labelKey string) (string, bool) {
	value, exists := result.Labels[labelKey]
	return value, exists
}

type alertsPrinter func(*[]*jsonInterceptor[alertResult])

func printAlertsAsText(alerts *[]*jsonInterceptor[alertResult]) {
	printMapping := func(mapping *rhobsmodels.LabelSet, mappingName string, forbiddenKeys map[string]struct{}) {
		keys := []string{}
		maxKeyWidth := 0
		for key := range *mapping {
			if _, ok := forbiddenKeys[key]; !ok {
				keys = append(keys, key)
				maxKeyWidth = max(maxKeyWidth, len(key))
			}
		}

		if len(keys) == 0 {
			return
		}

		sort.Strings(keys)

		fmt.Printf("  %s:\n", mappingName)
		for _, key := range keys {
			fmt.Printf("    %-*s: %s\n", maxKeyWidth, key, (*mapping)[key])
		}
	}

	consumedAnnotationNames := map[string]struct{}{
		"summary":     {},
		"description": {},
		"runbook_url": {},
		"html_url":    {},
	}

	for _, alert := range *alerts {
		fmt.Println(alert.decoded.Labels["alertname"], "-", alert.decoded.Annotations["summary"])
		fmt.Printf("  State      : %s\n", alert.decoded.Status.State)
		fmt.Printf("  Started at : %s\n", alert.decoded.StartsAt)
		fmt.Printf("  Description: %s\n", alert.decoded.Annotations["description"])
		fmt.Printf("  Runbook    : %s\n", alert.decoded.Annotations["runbook_url"])
		fmt.Printf("  Definition : %s\n", alert.decoded.Annotations["html_url"])
		printMapping(&alert.decoded.Annotations, "Other annotations", consumedAnnotationNames)
		printMapping(&alert.decoded.Labels, "All labels", map[string]struct{}{})

		fmt.Println()
	}
}

func printAlertsAsCsv(alerts *[]*jsonInterceptor[alertResult]) {
	annotationNamesSet := map[string]struct{}{}
	labelNamesSet := map[string]struct{}{}

	extractMappingKeySet := func(mapping *rhobsmodels.LabelSet, mappingKeySet *map[string]struct{}) {
		for key := range *mapping {
			(*mappingKeySet)[key] = struct{}{}
		}
	}

	for _, alert := range *alerts {
		extractMappingKeySet(&alert.decoded.Annotations, &annotationNamesSet)
		extractMappingKeySet(&alert.decoded.Labels, &labelNamesSet)
	}

	writer := csv.NewWriter(os.Stdout)

	header := []string{"NAME", "SUMMARY", "STATE", "STARTED_AT", "DESCRIPTION", "RUNBOOK", "DEFINITION"}
	for _, annotationName := range []string{"summary", "description", "runbook_url", "html_url"} {
		delete(annotationNamesSet, annotationName)
	}

	annotationNames := slices.Sorted(maps.Keys(annotationNamesSet))
	labelNames := slices.Sorted(maps.Keys(labelNamesSet))

	for _, annotationName := range annotationNames {
		header = append(header, annotationName+"_ANNOTATION")
	}

	header = append(header, labelNames...)

	err := writer.Write(header)
	if err != nil {
		log.Warnln("Failed to write CSV header:", err)
		return
	}

	for _, alert := range *alerts {
		row := []string{
			alert.decoded.Labels["alertname"],
			alert.decoded.Annotations["summary"],
			string(alert.decoded.Status.State),
			alert.decoded.StartsAt.String(),
			alert.decoded.Annotations["description"],
			alert.decoded.Annotations["runbook_url"],
			alert.decoded.Annotations["html_url"],
		}

		for _, annotationName := range annotationNames {
			row = append(row, alert.decoded.Annotations[annotationName])
		}

		for _, labelName := range labelNames {
			row = append(row, alert.decoded.Labels[labelName])
		}

		err := writer.Write(row)
		if err != nil {
			log.Warnln("Failed to write CSV row:", err)
			return
		}
	}

	writer.Flush()
}

func createAlertsPrinter(format AlertsFormat) alertsPrinter {
	switch format {
	case AlertsFormatJson:
		return printResultsAsJson[alertResult]
	case AlertsFormatCsv:
		return printAlertsAsCsv
	default:
		return printAlertsAsText
	}
}

func (f *RhobsFetcher) queryAlerts(ctx context.Context) (*[]*jsonInterceptor[alertResult], error) {
	client, err := f.getClient()
	if err != nil {
		return nil, err
	}

	log.Infoln("RHOBS cell:", f.RhobsCell)

	response, err := client.GetAlertsWithResponse(ctx, "hcp", &rhobsclient.GetAlertsParams{})

	if err != nil {
		return nil, fmt.Errorf("failed to send request to RHOBS: %v", err)
	}
	if response.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RHOBS query failed with status code: %d - body: %s", response.HTTPResponse.StatusCode, string(response.Body))
	}

	var formattedResponse getAlertsResponse
	if err := json.Unmarshal(response.Body, &formattedResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response from RHOBS: %v", err)
	}

	return &formattedResponse, nil
}

func (f *RhobsFetcher) PrintAlerts(ctx context.Context, format AlertsFormat, isPrintingClusterResultsOnly bool) error {
	alerts, err := f.queryAlerts(ctx)
	if err != nil {
		return err
	}

	alerts = filterMetricsResults(f, alerts, isPrintingClusterResultsOnly)
	createAlertsPrinter(format)(alerts)

	return nil
}

func (f *RhobsFetcher) QueryRules(ctx context.Context, ruleType string) (json.RawMessage, error) {
	client, err := f.getClient()
	if err != nil {
		return nil, err
	}

	log.Infoln("RHOBS cell:", f.RhobsCell)

	params := &rhobsclient.GetRulesParams{}
	if ruleType != "" {
		params.Type = &ruleType
	}

	response, err := client.GetRulesWithResponse(ctx, "hcp", params)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to RHOBS: %v", err)
	}
	if response.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RHOBS query failed with status code: %d - body: %s", response.HTTPResponse.StatusCode, string(response.Body))
	}

	return response.Body, nil
}

func (f *RhobsFetcher) QuerySilences(ctx context.Context) (json.RawMessage, error) {
	client, err := f.getClient()
	if err != nil {
		return nil, err
	}

	log.Infoln("RHOBS cell:", f.RhobsCell)

	response, err := client.GetSilencesWithResponse(ctx, "hcp", &rhobsclient.GetSilencesParams{})

	if err != nil {
		return nil, fmt.Errorf("failed to send request to RHOBS: %v", err)
	}
	if response.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RHOBS query failed with status code: %d - body: %s", response.HTTPResponse.StatusCode, string(response.Body))
	}

	return response.Body, nil
}

func (f *RhobsFetcher) createSilence(ctx context.Context, selectors *[]*alertsSelector, startTime, endTime time.Time, author, comment string) error {
	client, err := f.getClient()
	if err != nil {
		return err
	}

	log.Infoln("RHOBS cell:", f.RhobsCell)

	matchers := rhobsmodels.Matchers{}
	for _, selector := range *selectors {
		matchers = append(matchers, rhobsmodels.Matcher{
			Name:    selector.labelName,
			IsEqual: &selector.op.isEqual,
			IsRegex: selector.op.isRegexp,
			Value:   selector.labelValue,
		})
	}

	response, err := client.PostSilenceWithResponse(ctx, "hcp", &rhobsclient.PostSilenceParams{}, rhobsclient.PostSilenceJSONRequestBody{
		StartsAt:  startTime,
		EndsAt:    endTime,
		Comment:   comment,
		CreatedBy: author,
		Matchers:  matchers,
	})

	if err != nil {
		return fmt.Errorf("failed to send request to RHOBS: %v", err)
	}
	if response.HTTPResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("RHOBS query failed with status code: %d - body: %s", response.HTTPResponse.StatusCode, string(response.Body))
	}

	fmt.Println("DONE", string(response.Body))

	return nil
}

func (f *RhobsFetcher) DeleteSilence(ctx context.Context, silenceId uuid.UUID) error {
	client, err := f.getClient()
	if err != nil {
		return err
	}

	log.Infoln("RHOBS cell:", f.RhobsCell)

	response, err := client.DeleteSilenceWithResponse(ctx, "hcp", silenceId)

	if err != nil {
		return fmt.Errorf("failed to send request to RHOBS: %v", err)
	}
	if response.HTTPResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("RHOBS query failed with status code: %d - body: %s", response.HTTPResponse.StatusCode, string(response.Body))
	}

	fmt.Println("DONE", string(response.Body))

	return nil
}
