package utils

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/andygrunwald/go-jira"
	"github.com/openshift-online/ocm-cli/pkg/dump"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	v1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	backplaneapi "github.com/openshift/backplane-api/pkg/client"
	"github.com/openshift/osdctl/pkg/printer"
)

const (
	delimiter = ">> "
)

func PrintServiceLogs(serviceLogs []*v1.LogEntry, verbose bool, sinceDays int) {
	var name = fmt.Sprintf("Service Logs in the past %v days", sinceDays)
	fmt.Println(delimiter + name)

	if verbose {
		marshalledSLs, err := json.MarshalIndent(serviceLogs, "", "  ")
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Couldn't prepare service logs for printing: %v", err)
		}
		_ = dump.Pretty(os.Stdout, marshalledSLs)
	} else if len(serviceLogs) == 0 {
		fmt.Println("None")
	} else {
		// Non-verbose only prints the summaries
		for i, errorServiceLog := range serviceLogs {
			var serviceLogSummary string
			if errorServiceLog.InternalOnly() {
				internalServiceLogLines := strings.Split(errorServiceLog.Description(), "\n")
				if len(internalServiceLogLines) > 0 {
					// if the description is "", Split returns []
					serviceLogSummary = fmt.Sprintf("INT %s", internalServiceLogLines[0])
				} else {
					serviceLogSummary = errorServiceLog.Summary()
				}
			} else {
				serviceLogSummary = errorServiceLog.Summary()
			}
			serviceLogSummaryAbbreviated := serviceLogSummary[:int(math.Min(40, float64(len(serviceLogSummary))))]
			fmt.Printf("%d. %s (%s)\n", i, serviceLogSummaryAbbreviated, errorServiceLog.CreatedAt().Format(time.RFC3339))
		}
	}
}

func PrintPDAlerts(incidents map[string][]pd.Incident, serviceIDs []string) {
	var name = "PagerDuty Alerts"
	fmt.Println(delimiter + name)

	if len(serviceIDs) == 0 {
		fmt.Println("No PD Service Found")
		return
	}

	for _, ID := range serviceIDs {
		fmt.Printf("Service: https://redhat.pagerduty.com/service-directory/%s\n", ID)

		tableHasContent := false
		table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
		table.AddRow([]string{"Urgency", "Title", "Created At"})
		for _, incident := range incidents[ID] {
			table.AddRow([]string{incident.Urgency, incident.Title, incident.CreatedAt})
			tableHasContent = true
		}
		if tableHasContent {
			// Add empty row for readability
			table.AddRow([]string{})
			if err := table.Flush(); err != nil {
				fmt.Fprintf(os.Stderr, "Error printing %s - %s: %v\n", name, ID, err)
			}
		} else {
			fmt.Println("None")
		}
	}
}

func PrintJiraIssues(issues []jira.Issue) {
	var name = "OHSS Issues"
	fmt.Println(delimiter + name)

	for _, i := range issues {
		fmt.Printf("[%s|%s/browse/%s](%s/%s): %+v\n", i.Key, JiraBaseURL, i.Key, i.Fields.Type.Name, i.Fields.Priority.Name, i.Fields.Summary)
		fmt.Printf("- Created: %s\tStatus: %s\n", time.Time(i.Fields.Created).Format("2006-01-02 15:04"), i.Fields.Status.Name)
	}

	if len(issues) == 0 {
		fmt.Println("None")
	}
}

func PrintHandoverAnnouncements(issues []jira.Issue) {
	var name = "Related Handover Announcements"
	fmt.Println(delimiter + name)

	for _, i := range issues {
		fmt.Printf("[%s]: %+v\n", i.Key, i.Fields.Summary)
		fmt.Printf("- Link: %s/browse/%s\n\n", JiraBaseURL, i.Key)
	}

	if len(issues) == 0 {
		fmt.Println("None")
	}
}

func PrintLimitedSupportReasons(limitedSupportReasons []*cmv1.LimitedSupportReason) {
	var name = "Limited Support Status"
	fmt.Println(delimiter + name)

	// No reasons found, cluster is fully supported
	if len(limitedSupportReasons) == 0 {
		fmt.Printf("Fully supported\n")
		return
	}

	var limitedSupportOverridden = false
	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"Reason ID", "Summary", "Overridden (SUPPORTEX)", "Details"})
	for _, clusterLimitedSupportReason := range limitedSupportReasons {
		limitedSupportOverridden = limitedSupportOverridden || clusterLimitedSupportReason.Override().Enabled()
		table.AddRow([]string{
			clusterLimitedSupportReason.ID(),
			clusterLimitedSupportReason.Summary(),
			strconv.FormatBool(limitedSupportOverridden),
			clusterLimitedSupportReason.Details(),
		})
	}
	// Add empty row for readability
	table.AddRow([]string{})
	if err := table.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "Error printing %s: %v\n", name, err)
	}
}

func PrintClusterReports(reports *backplaneapi.ListReports) {
	fmt.Println(delimiter + "Cluster Reports 📒")

	if reports == nil || len(reports.Reports) == 0 {
		fmt.Println("None")
		return
	}

	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"Report ID", "Summary", "Created At"})
	for _, report := range reports.Reports {
		timeString := report.CreatedAt.Format(time.RFC3339)
		table.AddRow([]string{
			*report.ReportId,
			*report.Summary,
			timeString,
		})
	}

	if err := table.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "Error printing cluster reports: %v\n", err)
	}
}
