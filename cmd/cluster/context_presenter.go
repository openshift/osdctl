package cluster

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
)

// ClusterContextPresenter handles all output formatting for cluster context data.
// It separates presentation logic from data gathering and business logic.
type ClusterContextPresenter struct {
	writer io.Writer
}

// NewClusterContextPresenter creates a new presenter that writes to the given writer
func NewClusterContextPresenter(w io.Writer) *ClusterContextPresenter {
	return &ClusterContextPresenter{writer: w}
}

// Render renders the context data in the specified format
func (p *ClusterContextPresenter) Render(data *contextData, opts ContextOptions) error {
	switch opts.Output {
	case shortOutputConfigValue:
		return p.RenderShort(data, opts)
	case longOutputConfigValue:
		return p.RenderLong(data, opts)
	case jsonOutputConfigValue:
		return p.RenderJSON(data)
	default:
		return fmt.Errorf("unknown output format: %s", opts.Output)
	}
}

// RenderLong renders the full detailed output
func (p *ClusterContextPresenter) RenderLong(data *contextData, opts ContextOptions) error {
	data.printClusterHeader(p.writer)

	fmt.Fprintln(p.writer, strings.TrimSpace(data.Description))
	fmt.Fprintln(p.writer)
	printNetworkInfo(data, p.writer)
	fmt.Fprintln(p.writer)
	utils.PrintHandoverAnnouncements(data.HandoverAnnouncements)
	fmt.Fprintln(p.writer)
	utils.PrintLimitedSupportReasons(data.LimitedSupportReasons)
	fmt.Fprintln(p.writer)
	printJIRASupportExceptions(data.SupportExceptions, p.writer)
	fmt.Fprintln(p.writer)
	utils.PrintServiceLogs(data.ServiceLogs, opts.Verbose, opts.Days)
	fmt.Fprintln(p.writer)
	utils.PrintJiraIssues(data.JiraIssues)
	fmt.Fprintln(p.writer)
	utils.PrintPDAlerts(data.PdAlerts, data.pdServiceID)
	fmt.Fprintln(p.writer)

	if opts.FullScan {
		printHistoricalPDAlertSummary(data.HistoricalAlerts, data.pdServiceID, opts.Days, p.writer)
		fmt.Fprintln(p.writer)

		printCloudTrailLogs(data.CloudtrailEvents, p.writer)
		fmt.Fprintln(p.writer)
	}

	// Print other helpful links
	p.printOtherLinks(data, opts)
	fmt.Fprintln(p.writer)

	// Print Dynatrace URL
	printDynatraceResources(data, p.writer)

	// Print User Banned Details
	printUserBannedStatus(data, p.writer)

	// Print SDNtoOVN Migration Status
	printSDNtoOVNMigrationStatus(data, p.writer)

	return nil
}

// RenderShort renders the compact summary output
func (p *ClusterContextPresenter) RenderShort(data *contextData, opts ContextOptions) error {
	data.printClusterHeader(p.writer)

	highAlertCount := 0
	lowAlertCount := 0
	for _, alerts := range data.PdAlerts {
		for _, alert := range alerts {
			if strings.ToLower(alert.Urgency) == "high" {
				highAlertCount++
			} else {
				lowAlertCount++
			}
		}
	}

	historicalAlertsString := "N/A"
	historicalAlertsCount := 0
	if data.HistoricalAlerts != nil {
		for _, histAlerts := range data.HistoricalAlerts {
			for _, histAlert := range histAlerts {
				historicalAlertsCount += histAlert.Count
			}
		}
		historicalAlertsString = fmt.Sprintf("%d", historicalAlertsCount)
	}

	var numInternalServiceLogs int
	for _, serviceLog := range data.ServiceLogs {
		if serviceLog.InternalOnly() {
			numInternalServiceLogs++
		}
	}

	table := printer.NewTablePrinter(p.writer, 20, 1, 2, ' ')
	table.AddRow([]string{
		"Version",
		"Supported?",
		fmt.Sprintf("SLs (last %d d)", opts.Days),
		"Jira Tickets",
		"Current Alerts",
		fmt.Sprintf("Historical Alerts (last %d d)", opts.Days),
	})
	table.AddRow([]string{
		data.ClusterVersion,
		fmt.Sprintf("%t", len(data.LimitedSupportReasons) == 0),
		fmt.Sprintf("%d (%d internal)", len(data.ServiceLogs), numInternalServiceLogs),
		fmt.Sprintf("%d", len(data.JiraIssues)),
		fmt.Sprintf("H: %d | L: %d", highAlertCount, lowAlertCount),
		historicalAlertsString,
	})

	if err := table.Flush(); err != nil {
		return fmt.Errorf("error printing short output: %v", err)
	}

	return nil
}

// RenderJSON renders the output as JSON
func (p *ClusterContextPresenter) RenderJSON(data *contextData) error {
	jsonOut, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("can't marshal results to json: %v", err)
	}

	fmt.Fprintln(p.writer, string(jsonOut))
	return nil
}

// printOtherLinks prints external resource links
func (p *ClusterContextPresenter) printOtherLinks(data *contextData, opts ContextOptions) {
	var name string = "External resources"
	fmt.Fprintln(p.writer, delimiter+name)

	var ohssQueryURL = fmt.Sprintf("%[1]s/issues/?jql=project%%20%%3D%%22OpenShift%%20Hosted%%20SRE%%20Support%%22and%%20(%%22Cluster%%20ID%%22%%20~%%20%%20%%22%[2]s%%22OR%%22Cluster%%20ID%%22~%%22%[3]s%%22OR%%22description%%22~%%22%[2]s%%22OR%%22description%%22~%%22%[3]s%%22)",
		JiraBaseURL,
		data.ClusterID,
		data.ExternalClusterID)

	links := map[string]string{
		"OHSS Cards":        ohssQueryURL,
		"CCX dashboard":     fmt.Sprintf("https://kraken.psi.redhat.com/clusters/%s", data.ExternalClusterID),
		"Splunk Audit Logs": buildSplunkURL(data),
	}

	if data.pdServiceID != nil {
		for _, id := range data.pdServiceID {
			links[fmt.Sprintf("PagerDuty Service %s", id)] = fmt.Sprintf("https://redhat.pagerduty.com/service-directory/%s", id)
		}
	}

	// Sort, so it's always a predictable order
	var keys []string
	for k := range links {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	table := printer.NewTablePrinter(p.writer, 20, 1, 3, ' ')
	for _, link := range keys {
		table.AddRow([]string{link, strings.TrimSpace(links[link])})
	}

	if err := table.Flush(); err != nil {
		fmt.Fprintf(p.writer, "Error printing %s: %v\n", name, err)
	}
}

// buildSplunkURL constructs the appropriate Splunk URL based on cluster configuration
func buildSplunkURL(data *contextData) string {
	// Determine the relevant Splunk URL
	// at the time of this writing, the only region we will support in the near future will be the ap-southeast-1
	// region. Additionally, region-based clusters will ONLY be supported for HCP. Therefore, if we see a region
	// at all, we can assume that it's ap-southeast-1 and use that URL.
	if data.RegionID != "" {
		return buildHCPSplunkURL(SGPSplunkURL, data.OCMEnv, data.Cluster)
	}
	if data.Cluster != nil && data.Cluster.Hypershift().Enabled() {
		return buildHCPSplunkURL(HCPSplunkURL, data.OCMEnv, data.Cluster)
	} else {
		switch data.OCMEnv {
		case "production":
			return fmt.Sprintf(ClassicSplunkURL, "openshift_managed_audit", data.InfraID)
		case "stage":
			return fmt.Sprintf(ClassicSplunkURL, "openshift_managed_audit_stage", data.InfraID)
		default:
			return ""
		}
	}
}

func buildHCPSplunkURL(baseURL string, environment string, cluster *cmv1.Cluster) string {
	if cluster == nil {
		return ""
	}
	switch environment {
	case "production":
		return fmt.Sprintf(baseURL, "openshift_managed_hypershift_audit", "production", cluster.ID(), cluster.Name())
	case "stage":
		return fmt.Sprintf(baseURL, "openshift_managed_hypershift_audit_stage", "staging", cluster.ID(), cluster.Name())
	default:
		return ""
	}
}
