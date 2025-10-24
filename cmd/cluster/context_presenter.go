package cluster

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/andygrunwald/go-jira"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/cmd/dynatrace"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/provider/pagerduty"
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
	p.printNetworkInfo(data)
	fmt.Fprintln(p.writer)
	utils.PrintHandoverAnnouncements(data.HandoverAnnouncements)
	fmt.Fprintln(p.writer)
	utils.PrintLimitedSupportReasons(data.LimitedSupportReasons)
	fmt.Fprintln(p.writer)
	p.printJIRASupportExceptions(data.SupportExceptions)
	fmt.Fprintln(p.writer)
	utils.PrintServiceLogs(data.ServiceLogs, opts.Verbose, opts.Days)
	fmt.Fprintln(p.writer)
	utils.PrintJiraIssues(data.JiraIssues)
	fmt.Fprintln(p.writer)
	utils.PrintPDAlerts(data.PdAlerts, data.pdServiceID)
	fmt.Fprintln(p.writer)

	if opts.FullScan {
		p.printHistoricalPDAlertSummary(data.HistoricalAlerts, data.pdServiceID, opts.Days)
		fmt.Fprintln(p.writer)

		p.printCloudTrailLogs(data.CloudtrailEvents)
		fmt.Fprintln(p.writer)
	}

	// Print other helpful links
	p.printOtherLinks(data, opts)
	fmt.Fprintln(p.writer)

	// Print Dynatrace URL
	p.printDynatraceResources(data)

	// Print User Banned Details
	p.printUserBannedStatus(data)

	// Print SDNtoOVN Migration Status
	p.printSDNtoOVNMigrationStatus(data)

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

// printHistoricalPDAlertSummary prints a summary of historical PagerDuty alerts
func (p *ClusterContextPresenter) printHistoricalPDAlertSummary(incidentCounters map[string][]*pagerduty.IncidentOccurrenceTracker, serviceIDs []string, sinceDays int) {
	var name string = "PagerDuty Historical Alerts"
	fmt.Fprintln(p.writer, delimiter+name)

	for _, serviceID := range serviceIDs {

		if len(incidentCounters[serviceID]) == 0 {
			fmt.Fprintln(p.writer, "Service: https://redhat.pagerduty.com/service-directory/"+serviceID+": None")
			continue
		}

		fmt.Fprintln(p.writer, "Service: https://redhat.pagerduty.com/service-directory/"+serviceID+":")
		table := printer.NewTablePrinter(p.writer, 20, 1, 3, ' ')
		table.AddRow([]string{"Type", "Count", "Last Occurrence"})
		totalIncidents := 0
		for _, incident := range incidentCounters[serviceID] {
			table.AddRow([]string{incident.IncidentName, strconv.Itoa(incident.Count), incident.LastOccurrence})
			totalIncidents += incident.Count
		}

		// Add empty row for readability
		table.AddRow([]string{})
		if err := table.Flush(); err != nil {
			fmt.Fprintf(p.writer, "Error printing %s: %v\n", name, err)
		}

		fmt.Fprintln(p.writer, "\tTotal number of incidents [", totalIncidents, "] in [", sinceDays, "] days")
	}
}

// printJIRASupportExceptions prints JIRA support exception tickets
func (p *ClusterContextPresenter) printJIRASupportExceptions(issues []jira.Issue) {
	var name string = "Support Exceptions"
	fmt.Fprintln(p.writer, delimiter+name)

	for _, i := range issues {
		fmt.Fprintf(p.writer, "[%s](%s/%s): %+v [Status: %s]\n", i.Key, i.Fields.Type.Name, i.Fields.Priority.Name, i.Fields.Summary, i.Fields.Status.Name)
		fmt.Fprintf(p.writer, "- Link: %s/browse/%s\n\n", JiraBaseURL, i.Key)
	}

	if len(issues) == 0 {
		fmt.Fprintln(p.writer, "None")
	}
}

// printCloudTrailLogs prints potentially interesting CloudTrail events
func (p *ClusterContextPresenter) printCloudTrailLogs(events []*types.Event) {
	var name string = "Potentially interesting CloudTrail events"
	fmt.Fprintln(p.writer, delimiter+name)

	if events == nil {
		fmt.Fprintln(p.writer, "None")
		return
	}

	table := printer.NewTablePrinter(p.writer, 20, 1, 3, ' ')
	table.AddRow([]string{"EventId", "EventName", "Username", "EventTime"})
	for _, event := range events {
		if event.Username == nil {
			table.AddRow([]string{*event.EventId, *event.EventName, "", event.EventTime.String()})
		} else {
			table.AddRow([]string{*event.EventId, *event.EventName, *event.Username, event.EventTime.String()})
		}
	}
	// Add empty row for readability
	table.AddRow([]string{})
	if err := table.Flush(); err != nil {
		fmt.Fprintf(p.writer, "Error printing %s: %v\n", name, err)
	}
}

// printNetworkInfo prints network configuration details
func (p *ClusterContextPresenter) printNetworkInfo(data *contextData) {
	var name = "Network Info"
	fmt.Fprintln(p.writer, delimiter+name)

	table := printer.NewTablePrinter(p.writer, 20, 1, 3, ' ')
	table.AddRow([]string{"Network Type", data.NetworkType})
	table.AddRow([]string{"MachineCIDR", data.NetworkMachineCIDR})
	table.AddRow([]string{"ServiceCIDR", data.NetworkServiceCIDR})
	table.AddRow([]string{"Max Services", strconv.Itoa(data.NetworkMaxServices)})
	table.AddRow([]string{"PodCIDR", data.NetworkPodCIDR})
	table.AddRow([]string{"Host Prefix", strconv.Itoa(data.NetworkHostPrefix)})
	table.AddRow([]string{"Max Nodes (based on PodCIDR)", strconv.Itoa(data.NetworkMaxNodesFromPodCIDR)})
	table.AddRow([]string{"Max pods per node", strconv.Itoa(data.NetworkMaxPodsPerNode)})

	if err := table.Flush(); err != nil {
		fmt.Fprintf(p.writer, "Error printing %s: %v\n", name, err)
	}
}

// printDynatraceResources prints Dynatrace-related URLs and information
func (p *ClusterContextPresenter) printDynatraceResources(data *contextData) {
	var name string = "Dynatrace Details"
	fmt.Fprintln(p.writer, delimiter+name)

	links := map[string]string{
		"Dynatrace Tenant URL": data.DyntraceEnvURL,
		"Logs App URL":         data.DyntraceLogsURL,
	}

	// Sort, so it's always a predictable order
	var keys []string
	for k := range links {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	table := printer.NewTablePrinter(p.writer, 20, 1, 3, ' ')
	for _, link := range keys {
		url := strings.TrimSpace(links[link])
		if url == dynatrace.ErrUnsupportedCluster.Error() {
			fmt.Fprintln(p.writer, dynatrace.ErrUnsupportedCluster.Error())
			break
		} else if url != "" {
			table.AddRow([]string{link, url})
		}
	}

	if err := table.Flush(); err != nil {
		fmt.Fprintf(p.writer, "Error printing %s: %v\n", name, err)
	}
}

// printUserBannedStatus prints user ban status and details
func (p *ClusterContextPresenter) printUserBannedStatus(data *contextData) {
	var name string = "User Ban Details"
	fmt.Fprintln(p.writer, "\n"+delimiter+name)
	if data.UserBanned {
		fmt.Fprintln(p.writer, "User is banned")
		fmt.Fprintf(p.writer, "Ban code = %v\n", data.BanCode)
		fmt.Fprintf(p.writer, "Ban description = %v\n", data.BanDescription)
		if data.BanCode == BanCodeExportControlCompliance {
			fmt.Fprintln(p.writer, "User banned due to export control compliance.\nPlease follow the steps detailed here: https://github.com/openshift/ops-sop/blob/master/v4/alerts/UpgradeConfigSyncFailureOver4HrSRE.md#user-banneddisabled-due-to-export-control-compliance .")
		}
	} else {
		fmt.Fprintln(p.writer, "User is not banned")
	}
}

// printSDNtoOVNMigrationStatus prints the status of SDN to OVN migration
func (p *ClusterContextPresenter) printSDNtoOVNMigrationStatus(data *contextData) {
	name := "SDN to OVN Migration Status"
	fmt.Fprintln(p.writer, "\n"+delimiter+name)

	if data.SdnToOvnMigration != nil && data.MigrationStateValue == cmv1.ClusterMigrationStateValueInProgress {
		fmt.Fprintln(p.writer, "SDN to OVN migration is in progress")
		return
	}

	fmt.Fprintln(p.writer, "No active SDN to OVN migrations")
}
