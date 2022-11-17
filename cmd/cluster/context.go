package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	pd "github.com/PagerDuty/go-pagerduty"
	jira "github.com/andygrunwald/go-jira"
	"github.com/aws/aws-sdk-go/service/cloudtrail"
	"github.com/openshift-online/ocm-cli/pkg/dump"
	"github.com/openshift/osdctl/cmd/servicelog"
	sl "github.com/openshift/osdctl/internal/servicelog"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type contextOptions struct {
	output            string
	verbose           bool
	full              bool
	clusterID         string
	externalClusterID string
	baseDomain        string
	days              int
	pages             int
	oauthtoken        string
	usertoken         string
	externalID        string
	infraID           string
	awsProfile        string
}

const (
	JiraTokenConfigKey           = "jira_token"
	PagerDutyOauthTokenConfigKey = "pd_oauth_token"
	PagerDutyUserTokenConfigKey  = "pd_user_token"
)

// newCmdContext implements the context command to show the current context of a cluster
func newCmdContext() *cobra.Command {
	ops := newContextOptions()
	contextCmd := &cobra.Command{
		Use:               "context",
		Short:             "Shows the context of a specified cluster",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	contextCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Cluster ID")
	contextCmd.Flags().StringVarP(&ops.awsProfile, "profile", "p", "", "AWS Profile")
	contextCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")
	contextCmd.Flags().BoolVar(&ops.full, "full", false, "Run full suite of checks.")
	contextCmd.Flags().IntVarP(&ops.days, "days", "d", 30, "Command will display X days of Error SLs sent to the cluster. Days is set to 30 by default")
	contextCmd.Flags().IntVar(&ops.pages, "pages", 40, "Command will display X pages of Cloud Trail logs for the cluster. Pages is set to 40 by default")
	contextCmd.Flags().StringVar(&ops.oauthtoken, "oauthtoken", "", "Pass in PD oauthtoken directly. If not passed in, by default will read `pd_oauth_token` from ~/.config/osdctl")
	contextCmd.Flags().StringVar(&ops.usertoken, "usertoken", "", "Pass in PD usertoken directly. If not passed in, by default will read `pd_user_token` from ~/.config/osdctl")

	return contextCmd
}

func newContextOptions() *contextOptions {
	return &contextOptions{}
}

func (o *contextOptions) complete(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "Provide exactly one cluster ID")
	}

	if o.days < 1 {
		return fmt.Errorf("Cannot have a days value lower than 1")
	}

	// Create OCM client to talk to cluster API
	ocmClient := utils.CreateConnection()
	defer func() {
		if err := ocmClient.Close(); err != nil {
			fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	clusters := utils.GetClusters(ocmClient, args)
	if len(clusters) != 1 {
		return fmt.Errorf("unexpected number of clusters matched input. Expected 1 got %d", len(clusters))
	}
	o.clusterID = clusters[0].ID()
	o.externalClusterID = clusters[0].ExternalID()
	o.baseDomain = clusters[0].DNS().BaseDomain()
	o.externalID = clusters[0].ExternalID()
	o.infraID = clusters[0].InfraID()

	return nil
}

func (o *contextOptions) run() error {

	connection := utils.CreateConnection()
	defer connection.Close()

	err := printClusterInfo(o.clusterID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't print cluster info: %v\n", err)
		os.Exit(1)
	}

	limitedSupportReasons, err := utils.GetClusterLimitedSupportReasons(connection, o.clusterID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't retrieve cluster limited support reasons: %v\n", err)
		os.Exit(1)
	}

	// Check support status of cluster
	err = printSupportStatus(limitedSupportReasons)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't print support status: %v\n", err)
		os.Exit(1)
	}

	// Print the Servicelogs for this cluster
	err = o.printServiceLogs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't print service logs: %v\n", err)
		os.Exit(1)
	}

	err = o.printJiraCards()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't print jira cards: %v\n", err)
	}

	// Print all triggered and acknowledged pd alerts
	err = o.printPDAlerts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't print pagerduty alerts: %v\n", err)
		// Here we don't actually want to error out, this is to ensure that even if we don't have the
		// pd auth setup, we can still get the rest of the output.
	}

	// Print other helpful links
	err = o.printOtherLinks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't print other links: %v\n", err)
	}

	if o.full {
		err = o.printCloudTrailLogs()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Can't print cloudtrail: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println()
		fmt.Println("============================================================")
		fmt.Println("CloudTrail events for the Cluster")
		fmt.Println("============================================================")
		fmt.Println("Not polling cloudtrail logs, use --full flag to do so (must be logged into the correct hive to work).")
	}

	return nil
}

func printClusterInfo(clusterID string) error {

	fmt.Println("============================================================")
	fmt.Println("Cluster Info")
	fmt.Println("============================================================")

	cmd := "ocm describe cluster " + clusterID
	output, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		fmt.Println(string(output))
		fmt.Print(err)
		return err
	}
	fmt.Println(string(output))

	return nil
}

// printSupportStatus reports if a cluster is in limited support or fully supported.
func printSupportStatus(limitedSupportReasons []*utils.LimitedSupportReasonItem) error {

	fmt.Println("============================================================")
	fmt.Println("Limited Support Status")
	fmt.Println("============================================================")

	// No reasons found, cluster is fully supported
	if len(limitedSupportReasons) == 0 {
		fmt.Printf("Cluster is fully supported\n")
		fmt.Println()
		return nil
	}

	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"Reason ID", "Summary", "Details"})
	for _, clusterLimitedSupportReason := range limitedSupportReasons {
		table.AddRow([]string{clusterLimitedSupportReason.ID, clusterLimitedSupportReason.Summary, clusterLimitedSupportReason.Details})
	}
	// Add empty row for readability
	table.AddRow([]string{})
	table.Flush()

	return nil
}

func (o *contextOptions) printServiceLogs() error {

	// Get the SLs for the cluster
	slResponse, err := servicelog.FetchServiceLogs(o.clusterID)
	if err != nil {
		return err
	}

	var serviceLogs sl.ServiceLogShortList
	err = json.Unmarshal(slResponse.Bytes(), &serviceLogs)
	if err != nil {
		fmt.Printf("Failed to unmarshal the SL response %q\n", err)
		return err
	}

	// Parsing the relevant servicelogs
	// - We only care about SLs sent in the past 'o.days' days
	var errorServiceLogs []sl.ServiceLogShort
	for _, serviceLog := range serviceLogs.Items {
		// If the days since the SL was sent exceeds o.days days, we're not interested
		if (time.Since(serviceLog.CreatedAt).Hours() / 24) > float64(o.days) {
			continue
		}

		errorServiceLogs = append(errorServiceLogs, serviceLog)
	}

	fmt.Println("============================================================")
	fmt.Println("Service Logs sent in the past", o.days, "Days")
	fmt.Println("============================================================")

	if o.verbose {
		marshalledSLs, err := json.MarshalIndent(errorServiceLogs, "", "  ")
		if err != nil {
			return err
		}
		dump.Pretty(os.Stdout, marshalledSLs)
	} else {
		// Non verbose only prints the summaries
		for i, errorServiceLog := range errorServiceLogs {
			fmt.Printf("%d. %s (%s)\n", i, errorServiceLog.Summary, errorServiceLog.CreatedAt.Format(time.RFC3339))
		}
	}
	fmt.Println()

	return nil
}

func getPDUserClient(usertoken string) (*pd.Client, error) {
	if usertoken == "" {
		if !viper.IsSet(PagerDutyUserTokenConfigKey) {
			return nil, fmt.Errorf("key %s is not set in config file", PagerDutyUserTokenConfigKey)
		}
		usertoken = viper.GetString(PagerDutyUserTokenConfigKey)
	}
	return pd.NewClient(usertoken), nil
}

func getPDOauthClient(oauthtoken string) (*pd.Client, error) {
	if oauthtoken == "" {
		if !viper.IsSet(PagerDutyOauthTokenConfigKey) {
			return nil, fmt.Errorf("key %s is not set in config file", PagerDutyOauthTokenConfigKey)
		}
		oauthtoken = viper.GetString(PagerDutyOauthTokenConfigKey)
	}
	return pd.NewOAuthClient(oauthtoken), nil
}

func GetPagerdutyClient(usertoken string, oauthtoken string) (*pd.Client, error) {
	client, err := getPDUserClient(usertoken)
	if client != nil {
		return client, err
	}
	client, err = getPDOauthClient(oauthtoken)
	if err != nil {
		return nil, fmt.Errorf("Failed to create both user and oauth clients for pd")
	}
	return client, err
}

func getPDSeviceID(pdClient *pd.Client, ctx context.Context, baseDomain string) (string, error) {
	lsResponse, err := pdClient.ListServicesWithContext(ctx, pd.ListServiceOptions{Query: baseDomain})

	if err != nil {
		fmt.Printf("Failed to ListServicesWithContext %q\n", err)
		return "", err
	}

	if len(lsResponse.Services) != 1 {
		return "", fmt.Errorf("unexpected number of services matched input. Expected 1 got %d", len(lsResponse.Services))
	}

	return lsResponse.Services[0].ID, nil
}

func printCurrentPDAlerts(pdClient *pd.Client, ctx context.Context, serviceID string) error {
	liResponse, err := pdClient.ListIncidentsWithContext(
		ctx,
		pd.ListIncidentsOptions{
			ServiceIDs: []string{serviceID},
			Statuses:   []string{"triggered", "acknowledged"},
		},
	)
	if err != nil {
		fmt.Printf("Failed to ListIncidentsWithContext %q\n", err)
		return err
	}

	fmt.Println("============================================================")
	fmt.Println("Current Pagerduty Alerts for the Cluster")
	fmt.Println("============================================================")
	fmt.Printf("Link to PD Service: https://redhat.pagerduty.com/service-directory/%s\n", serviceID)
	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"Urgency", "Title", "Created At"})
	for _, incident := range liResponse.Incidents {
		table.AddRow([]string{incident.Urgency, incident.Title, incident.CreatedAt})
	}
	// Add empty row for readability
	table.AddRow([]string{})
	err = table.Flush()
	if err != nil {
		fmt.Println("error while flushing table: ", err.Error())
		return err
	}
	return nil
}

func printHistoricalPDAlertSummary(pdClient *pd.Client, ctx context.Context, serviceID string) error {

	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("Historical Pagerduty Alert Summary")
	fmt.Println("============================================================")
	fmt.Printf("Link to PD Service: https://redhat.pagerduty.com/service-directory/%s\n", serviceID)

	var currentOffset uint
	var limit uint = 100
	var incidents []pd.Incident
	fmt.Println("Pulling historical pd data")
	for currentOffset = 0; true; currentOffset += limit {
		print(".")
		// pd defaults pulling the past month of data, which is enough for us to work with
		liResponse, err := pdClient.ListIncidentsWithContext(
			ctx,
			pd.ListIncidentsOptions{
				ServiceIDs: []string{serviceID},
				Statuses:   []string{"resolved", "triggered", "acknowledged"},
				Offset:     currentOffset,
				Limit:      limit,
				SortBy:     "created_at:desc",
			},
		)

		if err != nil {
			return err
		}

		if len(liResponse.Incidents) == 0 {
			break
		}

		incidents = append(incidents, liResponse.Incidents...)
	}
	println()

	type occurrenceTracker struct {
		incidentCount  int
		lastOccurrence string
	}

	incidentCounter := make(map[string]*occurrenceTracker)

	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"Type", "Count", "Last Occurrence"})

	var incidentKeys []string
	for _, incident := range incidents {
		title := strings.Split(incident.Title, " ")[0]
		if _, found := incidentCounter[title]; found {
			incidentCounter[title].incidentCount++

			// Compare current incident timestamp vs our previous 'latest occurrence', and save the most recent.
			currentLastOccurence, err := time.Parse(time.RFC3339, incidentCounter[title].lastOccurrence)
			if err != nil {
				fmt.Printf("Failed to parse time %q\n", err)
				return err
			}

			incidentCreatedAt, err := time.Parse(time.RFC3339, incident.CreatedAt)
			if err != nil {
				fmt.Printf("Failed to parse time %q\n", err)
				return err
			}

			// We want to see when the latest occurrence was
			if incidentCreatedAt.After(currentLastOccurence) {
				incidentCounter[title].lastOccurrence = incident.CreatedAt
			}

		} else {
			// First time encountering this incident type
			incidentCounter[title] = &occurrenceTracker{
				incidentCount:  1,
				lastOccurrence: incident.CreatedAt,
			}

			incidentKeys = append(incidentKeys, title)
		}
	}

	if len(incidentKeys) == 0 {
		fmt.Println("No historical pagerduty data")
		fmt.Println()
		return nil
	}

	sort.SliceStable(incidentKeys, func(i, j int) bool {
		return incidentCounter[incidentKeys[i]].incidentCount > incidentCounter[incidentKeys[j]].incidentCount
	})

	for _, k := range incidentKeys {
		table.AddRow([]string{k, strconv.Itoa(incidentCounter[k].incidentCount), incidentCounter[k].lastOccurrence})
	}

	// Add empty row for readability
	table.AddRow([]string{})
	err := table.Flush()
	if err != nil {
		fmt.Println("error while flushing table: ", err.Error())
		return err
	}

	totalIncidents := len(incidents)
	oldestIncidentTimestamp, err := time.Parse(time.RFC3339, incidents[totalIncidents-1].CreatedAt)
	if err != nil {
		fmt.Printf("Failed to parse time %q\n", err)
		return err
	}
	oldestIncidentTimeInDays := int(time.Since(oldestIncidentTimestamp).Hours() / 24)
	fmt.Println("Total number of incidents [", totalIncidents, "] in [", oldestIncidentTimeInDays, "] days")

	return nil
}

func (o *contextOptions) printJiraCards() error {

	if !viper.IsSet(JiraTokenConfigKey) {
		return fmt.Errorf("key %s is not set in config file", JiraTokenConfigKey)
	}

	jiratoken := viper.GetString(JiraTokenConfigKey)

	tp := jira.PATAuthTransport{
		Token: jiratoken,
	}

	jiraClient, _ := jira.NewClient(tp.Client(), "https://issues.redhat.com/")

	jql := fmt.Sprintf(
		`(project = "OpenShift Hosted SRE Support" AND "Cluster ID" ~ "%s") 
		OR (project = "OpenShift Hosted SRE Support" AND "Cluster ID" ~ "%s") 
		ORDER BY priority DESC, created DESC`,
		o.externalClusterID,
		o.clusterID,
	)

	issues, _, err := jiraClient.Issue.Search(jql, nil)
	if err != nil {
		fmt.Printf("Failed to search for jira issues %q\n", err)
		return err
	}

	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("Cluster JIRAs")
	fmt.Println("============================================================")

	for _, i := range issues {
		fmt.Printf("[%s](%s/%s): %+v [Status: %s]\n", i.Key, i.Fields.Type.Name, i.Fields.Priority.Name, i.Fields.Summary, i.Fields.Status.Name)
		fmt.Printf("- Link: https://issues.redhat.com/browse/%s\n\n", i.Key)
	}
	return nil
}

func (o *contextOptions) printPDAlerts() error {

	pdClient, err := GetPagerdutyClient(o.usertoken, o.oauthtoken)
	if err != nil {
		fmt.Println("error getting pd client: ", err.Error())
		return err
	}

	ctx := context.TODO()
	serviceID, err := getPDSeviceID(pdClient, ctx, o.baseDomain)
	if err != nil {
		fmt.Println("error getting pd service id: ", err.Error())
		return err
	}

	err = printCurrentPDAlerts(pdClient, ctx, serviceID)
	if err != nil {
		fmt.Println("error calling printCurrentPDAlerts: ", err.Error())
		return err
	}

	err = printHistoricalPDAlertSummary(pdClient, ctx, serviceID)
	if err != nil {
		fmt.Println("error calling printHistoricalPDAlertSummary: ", err.Error())
		return err
	}

	return nil
}

func (o *contextOptions) printOtherLinks() error {
	fmt.Println("============================================================")
	fmt.Println("External resources containing related cluster data")
	fmt.Println("============================================================")
	fmt.Printf("Link to Splunk audit logs (set time in Splunk): https://osdsecuritylogs.splunkcloud.com/en-US/app/search/search?q=search%%20index%%3D%%22openshift_managed_audit%%22%%20clusterid%%3D%%22%s%%22\n\n", o.infraID)
	fmt.Printf("Link to OHSS tickets: https://issues.redhat.com/issues/?jql=project%%20%%3D%%20OHSS%%20and%%20(%%22Cluster%%20ID%%22%%20~%%20%%20%%22%s%%22%%20OR%%20%%22Cluster%%20ID%%22%%20~%%20%%22%s%%22)\n\n", o.clusterID, o.externalID)
	fmt.Printf("Link to CCX dashboard: https://kraken.psi.redhat.com/clusters/%s\n\n", o.externalID)

	return nil
}

func (o *contextOptions) printCloudTrailLogs() error {

	awsJumpClient, err := osdCloud.GenerateAWSClientForCluster(o.awsProfile, o.clusterID)
	if err != nil {
		return err
	}

	foundEvents := []*cloudtrail.Event{}
	var eventSearchInput = cloudtrail.LookupEventsInput{}

	fmt.Println("Pulling and filtering the past", o.pages, "pages of Cloudtrail data")
	for counter := 0; counter <= o.pages; counter++ {
		print(".")
		cloudTrailEvents, err := awsJumpClient.LookupEvents(&eventSearchInput)
		if err != nil {
			return err
		}

		foundEvents = append(foundEvents, cloudTrailEvents.Events...)

		// for pagination
		eventSearchInput.NextToken = cloudTrailEvents.NextToken
		if cloudTrailEvents.NextToken == nil {
			break
		}
	}
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("Potentially interesting CloudTrail events for the Cluster")
	fmt.Println("============================================================")

	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"EventId", "EventName", "Username", "EventTime"})
	for _, event := range foundEvents {
		if skippableEvent(*event.EventName) {
			continue
		}
		if event.Username == nil {
			table.AddRow([]string{*event.EventId, *event.EventName, "", event.EventTime.String()})
		} else {
			if strings.Contains(*event.Username, "RH-SRE-") {
				continue
			}
			table.AddRow([]string{*event.EventId, *event.EventName, *event.Username, event.EventTime.String()})
		}

	}
	// Add empty row for readability
	table.AddRow([]string{})
	err = table.Flush()
	if err != nil {
		fmt.Println("error while flushing table: ", err.Error())
		return err
	}

	return nil
}

// These are a list of skippable aws event types, as they won't indicate any modification on the customer's side.
func skippableEvent(eventName string) bool {
	skippableList := []string{
		"Get",
		"List",
		"Describe",
		"AssumeRole",
		"Encrypt",
		"Decrypt",
		"LookupEvents",
		"GenerateDataKey",
	}

	for _, skipword := range skippableList {
		if strings.Contains(eventName, skipword) {
			return true
		}
	}
	return false
}
