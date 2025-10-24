package cluster

import (
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	v1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"

	"github.com/openshift/osdctl/cmd/servicelog"

	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/andygrunwald/go-jira"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/cmd/dynatrace"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/provider/pagerduty"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	JiraBaseURL                   = "https://issues.redhat.com"
	JiraTokenRegistrationPath     = "/secure/ViewProfile.jspa?selectedTab=com.atlassian.pats.pats-plugin:jira-user-personal-access-tokens" // #nosec G101
	PagerDutyTokenRegistrationUrl = "https://martindstone.github.io/PDOAuth/"                                                              // #nosec G101
	ClassicSplunkURL              = "https://osdsecuritylogs.splunkcloud.com/en-US/app/search/search?q=search%%20index%%3D%%22%s%%22%%20clusterid%%3D%%22%s%%22\n\n"
	HCPSplunkURL                  = "https://osdsecuritylogs.splunkcloud.com/en-US/app/search/search?q=search%%20index%%3D%%22%s%%22%%20annotations.managed.openshift.io%%2Fhosted-cluster-id%%3Docm-%s-%s-%s\n\n"
	SGPSplunkURL                  = "https://osd-ase1.splunkcloud.com/en-US/app/search/search?q=search%%20index%%3D%%22%s%%22%%20annotations.managed.openshift.io%%2Fhosted-cluster-id%%3Docm-%s-%s-%s\n\n"
	shortOutputConfigValue        = "short"
	longOutputConfigValue         = "long"
	jsonOutputConfigValue         = "json"
	delimiter                     = ">> "
)

// ContextOptions is a pure configuration struct containing all the parameters
// needed to query cluster context information. It has no methods except validation.
type ContextOptions struct {
	ClusterID  string
	Days       int
	Pages      int
	FullScan   bool
	Verbose    bool
	Output     string
	AWSProfile string
	OAuthToken string
	UserToken  string
	JiraToken  string
	TeamIDs    []string
}

// Validate ensures the query options are valid
func (o ContextOptions) Validate() error {
	if o.Days < 1 {
		return fmt.Errorf("cannot have a days value lower than 1")
	}
	return nil
}

// ContextCache holds the runtime state and cluster information needed during execution.
type ContextCache struct {
	cluster *cmv1.Cluster

	clusterID         string
	externalClusterID string
	baseDomain        string
	organizationID    string
	infraID           string
	regionID          string

	// Query options - configuration for the context query
	queryOpts ContextOptions
}

type contextData struct {
	// Cluster info
	ClusterName       string
	ClusterVersion    string
	ClusterID         string
	ExternalClusterID string
	InfraID           string

	// Current OCM environment (e.g., "production" or "stage")
	OCMEnv string

	// RegionID (used for region-locked clusters)
	RegionID string

	// Cluster object for advanced queries
	Cluster *cmv1.Cluster

	// Dynatrace Environment URL and Logs URL
	DyntraceEnvURL  string
	DyntraceLogsURL string

	// limited Support Status
	LimitedSupportReasons []*cmv1.LimitedSupportReason
	// Service Logs
	ServiceLogs []*v1.LogEntry

	// Jira Cards
	JiraIssues            []jira.Issue
	HandoverAnnouncements []jira.Issue
	SupportExceptions     []jira.Issue

	// PD Alerts
	pdServiceID      []string
	PdAlerts         map[string][]pd.Incident
	HistoricalAlerts map[string][]*pagerduty.IncidentOccurrenceTracker

	// CloudTrail Logs
	CloudtrailEvents []*types.Event

	// OCM Cluster description
	Description string

	// User Banned Information
	UserBanned     bool
	BanCode        string
	BanDescription string

	// Network data
	NetworkType                string
	NetworkMachineCIDR         string
	NetworkServiceCIDR         string
	NetworkPodCIDR             string
	NetworkHostPrefix          int
	NetworkMaxNodesFromPodCIDR int
	NetworkMaxPodsPerNode      int
	NetworkMaxServices         int

	// Migration data
	SdnToOvnMigration   *cmv1.SdnToOvnClusterMigration
	MigrationStateValue cmv1.ClusterMigrationStateValue
}

// newCmdContext implements the context command to show the current context of a cluster
func newCmdContext() *cobra.Command {
	var queryOpts ContextOptions

	contextCmd := &cobra.Command{
		Use:               "context --cluster-id <cluster-identifier>",
		Short:             "Shows the context of a specified cluster",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate query options
			if err := queryOpts.Validate(); err != nil {
				return err
			}

			// Create and setup context options with query configuration
			ops := newContextOptions(queryOpts)
			err := ops.setup()
			if err != nil {
				return err
			}

			return ops.run()
		},
	}

	contextCmd.Flags().StringVarP(&queryOpts.ClusterID, "cluster-id", "C", "", "Provide internal ID of the cluster")
	_ = contextCmd.MarkFlagRequired("cluster-id")

	contextCmd.Flags().StringVarP(&queryOpts.Output, "output", "o", "long", "Valid formats are ['long', 'short', 'json']. Output is set to 'long' by default")
	contextCmd.Flags().StringVarP(&queryOpts.AWSProfile, "profile", "p", "", "AWS Profile")
	contextCmd.Flags().BoolVarP(&queryOpts.Verbose, "verbose", "", false, "Verbose output")
	contextCmd.Flags().BoolVar(&queryOpts.FullScan, "full", false, "Run full suite of checks.")
	contextCmd.Flags().IntVarP(&queryOpts.Days, "days", "d", 30, "Command will display X days of Error SLs sent to the cluster. Days is set to 30 by default")
	contextCmd.Flags().IntVar(&queryOpts.Pages, "pages", 40, "Command will display X pages of Cloud Trail logs for the cluster. Pages is set to 40 by default")
	contextCmd.Flags().StringVar(&queryOpts.OAuthToken, "oauthtoken", "", fmt.Sprintf("Pass in PD oauthtoken directly. If not passed in, by default will read `pd_oauth_token` from ~/.config/%s.\nPD OAuth tokens can be generated by visiting %s", osdctlConfig.ConfigFileName, PagerDutyTokenRegistrationUrl))
	contextCmd.Flags().StringVar(&queryOpts.UserToken, "usertoken", "", fmt.Sprintf("Pass in PD usertoken directly. If not passed in, by default will read `pd_user_token` from ~/config/%s", osdctlConfig.ConfigFileName))
	contextCmd.Flags().StringVar(&queryOpts.JiraToken, "jiratoken", "", fmt.Sprintf("Pass in the Jira access token directly. If not passed in, by default will read `jira_token` from ~/.config/%s.\nJira access tokens can be registered by visiting %s/%s", osdctlConfig.ConfigFileName, JiraBaseURL, JiraTokenRegistrationPath))
	contextCmd.Flags().StringArrayVarP(&queryOpts.TeamIDs, "team-ids", "t", []string{}, fmt.Sprintf("Pass in PD team IDs directly to filter the PD Alerts by team. Can also be defined as `team_ids` in ~/.config/%s\nWill show all PD Alerts for all PD service IDs if none is defined", osdctlConfig.ConfigFileName))
	return contextCmd
}

func newContextOptions(queryOpts ContextOptions) *ContextCache {
	return &ContextCache{
		queryOpts: queryOpts,
	}
}

func (o *ContextCache) setup() error {
	// Create OCM client to talk to cluster API
	defer utils.StartDelayTracker(o.queryOpts.Verbose, "OCM Clusters").End()
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer func() {
		if err := ocmClient.Close(); err != nil {
			fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	// Use the clusterID flag value instead of args
	clusterArgs := []string{o.queryOpts.ClusterID}
	clusters := utils.GetClusters(ocmClient, clusterArgs)
	if len(clusters) != 1 {
		return fmt.Errorf("unexpected number of clusters matched input. Expected 1 got %d", len(clusters))
	}

	o.cluster = clusters[0]
	o.clusterID = o.cluster.ID()
	o.externalClusterID = o.cluster.ExternalID()
	o.baseDomain = o.cluster.DNS().BaseDomain()
	o.infraID = o.cluster.InfraID()

	if o.queryOpts.UserToken == "" {
		o.queryOpts.UserToken = viper.GetString(pagerduty.PagerDutyUserTokenConfigKey)
	}

	if o.queryOpts.OAuthToken == "" {
		o.queryOpts.OAuthToken = viper.GetString(pagerduty.PagerDutyOauthTokenConfigKey)
	}

	sub, err := utils.GetSubFromClusterID(ocmClient, *o.cluster)
	if err != nil {
		fmt.Printf("Failed to get Subscription for cluster %s - err: %q", o.clusterID, err)
	}

	o.organizationID = sub.OrganizationID()
	o.regionID = sub.RhRegionID()

	return nil
}

func (o *ContextCache) run() error {
	currentData, dataErrors := o.generateContextData()
	if currentData == nil {
		fmt.Fprintf(os.Stderr, "Failed to query cluster info: %+v", dataErrors)
		os.Exit(1)
	}

	if len(dataErrors) > 0 {
		fmt.Fprintf(os.Stderr, "Encountered Errors during data collection. Displayed data may be incomplete: \n")
		for _, dataError := range dataErrors {
			fmt.Fprintf(os.Stderr, "\t%v\n", dataError)
		}
	}

	// Use the presenter to render output
	presenter := NewClusterContextPresenter(os.Stdout)
	return presenter.Render(currentData, o.queryOpts)
}

func GenerateContextData(clusterId string) (string, []error) {
	queryOpts := ContextOptions{
		ClusterID: clusterId,
		Days:      30,
		Pages:     40,
		FullScan:  false,
		Verbose:   false,
		Output:    jsonOutputConfigValue,
	}
	contextOptions := newContextOptions(queryOpts)
	err := contextOptions.setup()
	if err != nil {
		return "", []error{err}
	}

	contextData, errs := contextOptions.generateContextData()

	builder := &strings.Builder{}
	presenter := NewClusterContextPresenter(builder)
	presenter.Render(contextData, queryOpts)
	return builder.String(), errs
}

// generateContextData Creates a contextData struct that contains all the
// cluster context information requested by the contextOptions. if a certain
// data point can not be queried, the appropriate field will be null and the
// errors array will contain information about the error. The first return
// value will only be nil, if this function fails to get basic cluster
// information. The second return value will *never* be nil, but instead have a
// length of 0 if no errors occurred
func (o *ContextCache) generateContextData() (*contextData, []error) {
	data := &contextData{}
	errors := []error{}

	wg := sync.WaitGroup{}

	// For PD query dependencies
	pdwg := sync.WaitGroup{}
	var skipPagerDutyCollection bool
	pdProvider, err := pagerduty.NewClient().
		WithUserToken(o.queryOpts.UserToken).
		WithOauthToken(o.queryOpts.OAuthToken).
		WithBaseDomain(o.baseDomain).
		WithTeamIdList(viper.GetStringSlice(pagerduty.PagerDutyTeamIDsKey)).
		Init()
	if err != nil {
		skipPagerDutyCollection = true
		errors = append(errors, fmt.Errorf("skipping PagerDuty context collection: %v", err))
	}

	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return nil, []error{err}
	}
	defer ocmClient.Close()
	// Normally the o.cluster would be set by complete function, but in case we want to call this function
	// in an other context, we can make sure o.cluster is set properly from o.clusterID
	if o.cluster == nil {
		cluster, err := utils.GetCluster(ocmClient, o.clusterID)
		if err != nil {
			errors = append(errors, err)
			return nil, errors
		}
		o.cluster = cluster
	}

	data.ClusterName = o.cluster.Name()
	data.ClusterID = o.clusterID
	data.ExternalClusterID = o.externalClusterID
	data.InfraID = o.infraID
	data.RegionID = o.regionID
	data.Cluster = o.cluster
	data.ClusterVersion = o.cluster.Version().RawID()
	data.OCMEnv = utils.GetCurrentOCMEnv(ocmClient)

	// network info fetch and calculations
	var clusterNetwork = o.cluster.Network()
	var ok bool
	var podNetwork *net.IPNet
	var serviceNetwork *net.IPNet

	data.NetworkType = clusterNetwork.Type()
	data.NetworkMachineCIDR, ok = clusterNetwork.GetMachineCIDR()
	if !ok {
		errors = append(errors, fmt.Errorf("missing Machine CIDR in OCM Cluster"))
		return nil, errors
	}
	data.NetworkServiceCIDR = clusterNetwork.ServiceCIDR()
	data.NetworkPodCIDR = clusterNetwork.PodCIDR()
	data.NetworkHostPrefix = clusterNetwork.HostPrefix()

	//max possible nodes from hostprefix

	_, podNetwork, err = net.ParseCIDR(data.NetworkPodCIDR)
	if err != nil {
		errors = append(errors, err)
		return nil, errors
	}
	var b, max = podNetwork.Mask.Size()
	data.NetworkMaxNodesFromPodCIDR = int(math.Pow(float64(2), float64(data.NetworkHostPrefix-b)))

	//max pods per node
	data.NetworkMaxPodsPerNode = int(math.Pow(float64(2), float64(max-data.NetworkHostPrefix)))

	//max services

	_, serviceNetwork, err = net.ParseCIDR(data.NetworkServiceCIDR)
	if err != nil {
		errors = append(errors, err)
		return nil, errors
	}
	b, max = serviceNetwork.Mask.Size()
	data.NetworkMaxServices = int(math.Pow(float64(2), float64(max-b))) - 2 // minus 2: API and DNS service

	GetLimitedSupport := func() {
		defer wg.Done()
		defer utils.StartDelayTracker(o.queryOpts.Verbose, "Limited Support reasons").End()
		limitedSupportReasons, err := utils.GetClusterLimitedSupportReasons(ocmClient, o.clusterID)
		if err != nil {
			errors = append(errors, fmt.Errorf("error while getting Limited Support status reasons: %v", err))
		} else {
			data.LimitedSupportReasons = append(data.LimitedSupportReasons, limitedSupportReasons...)
		}
	}

	GetServiceLogs := func() {
		defer wg.Done()
		defer utils.StartDelayTracker(o.queryOpts.Verbose, "Service Logs").End()
		timeToCheckSvcLogs := time.Now().AddDate(0, 0, -o.queryOpts.Days)
		data.ServiceLogs, err = servicelog.GetServiceLogsSince(o.clusterID, timeToCheckSvcLogs, false, false)
		if err != nil {
			errors = append(errors, fmt.Errorf("error while getting the service logs: %v", err))
		}
	}

	GetBannedUser := func() {
		defer wg.Done()
		defer utils.StartDelayTracker(o.queryOpts.Verbose, "Check Banned User").End()
		subscription, err := utils.GetSubscription(ocmClient, data.ClusterID)
		if err != nil {
			errors = append(errors, fmt.Errorf("error while getting subscripton %v", err))
		}
		creator, err := utils.GetAccount(ocmClient, subscription.Creator().ID())
		if err != nil {
			errors = append(errors, fmt.Errorf("error while checking if user is banned %v", err))
		}
		data.UserBanned = creator.Banned()
		data.BanCode = creator.BanCode()
		data.BanDescription = creator.BanDescription()
	}

	GetJiraIssues := func() {
		defer wg.Done()
		defer utils.StartDelayTracker(o.queryOpts.Verbose, "Jira Issues").End()
		data.JiraIssues, err = utils.GetJiraIssuesForCluster(o.clusterID, o.externalClusterID, o.queryOpts.JiraToken)
		if err != nil {
			errors = append(errors, fmt.Errorf("error while getting the open jira tickets: %v", err))
		}
	}

	GetHandoverAnnouncements := func() {
		defer wg.Done()
		defer utils.StartDelayTracker(o.queryOpts.Verbose, "Handover Announcements").End()
		org, err := utils.GetOrganization(ocmClient, o.clusterID)
		if err != nil {
			fmt.Printf("Failed to get Subscription for cluster %s - err: %q", o.clusterID, err)
		}

		productID := o.cluster.Product().ID()
		data.HandoverAnnouncements, err = utils.GetRelatedHandoverAnnouncements(o.clusterID, o.externalClusterID, o.queryOpts.JiraToken, org.Name(), productID, o.cluster.Hypershift().Enabled(), o.cluster.Version().RawID())
		if err != nil {
			errors = append(errors, fmt.Errorf("error while getting the open jira tickets: %v", err))
		}
	}

	GetSupportExceptions := func() {
		defer wg.Done()
		defer utils.StartDelayTracker(o.queryOpts.Verbose, "Support Exceptions").End()
		data.SupportExceptions, err = utils.GetJiraSupportExceptionsForOrg(o.organizationID, o.queryOpts.JiraToken)
		if err != nil {
			errors = append(errors, fmt.Errorf("error while getting support exceptions: %v", err))
		}
	}

	GetDynatraceDetails := func() {
		var clusterID string = o.clusterID
		defer wg.Done()
		defer utils.StartDelayTracker(o.queryOpts.Verbose, "Dynatrace URL").End()

		hcpCluster, err := dynatrace.FetchClusterDetails(clusterID)
		if err != nil {
			if err == dynatrace.ErrUnsupportedCluster {
				data.DyntraceEnvURL = dynatrace.ErrUnsupportedCluster.Error()
			} else {
				errors = append(errors, fmt.Errorf("failed to acquire cluster details %v", err))
				data.DyntraceEnvURL = "Failed to fetch Dynatrace URL"
			}
			return
		}
		query, err := dynatrace.GetQuery(hcpCluster, time.Time{}, time.Time{}, 1) // passing nil from/to values to use --since behaviour
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to build query for Dynatrace %v", err))
			data.DyntraceEnvURL = fmt.Sprintf("Failed to build Dynatrace query: %v", err)
			return
		}
		queryTxt := query.Build()
		data.DyntraceEnvURL = hcpCluster.DynatraceURL
		data.DyntraceLogsURL, err = dynatrace.GetLinkToWebConsole(hcpCluster.DynatraceURL, "now()-10h", "now()", queryTxt)
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to get url: %v", err))
		}

	}

	GetPagerDutyAlerts := func() {
		pdwg.Add(1)
		defer wg.Done()
		defer pdwg.Done()

		if skipPagerDutyCollection {
			return
		}

		delayTracker := utils.StartDelayTracker(o.queryOpts.Verbose, "PagerDuty Service")
		data.pdServiceID, err = pdProvider.GetPDServiceIDs()
		if err != nil {
			errors = append(errors, fmt.Errorf("error getting PD Service ID: %v", err))
		}
		delayTracker.End()

		defer utils.StartDelayTracker(o.queryOpts.Verbose, "current PagerDuty Alerts").End()
		data.PdAlerts, err = pdProvider.GetFiringAlertsForCluster(data.pdServiceID)
		if err != nil {
			errors = append(errors, fmt.Errorf("error while getting current PD Alerts: %v", err))
		}
	}

	GetMigrationInfo := func() {
		defer wg.Done()
		defer utils.StartDelayTracker(o.queryOpts.Verbose, "Migration Info").End()

		migrationResponse, err := utils.GetMigration(ocmClient, o.clusterID)
		if err != nil {
			errors = append(errors, fmt.Errorf("error while getting migration info: %v", err))
			return
		}

		sdntoovnmigration, ok := migrationResponse.GetSdnToOvn()
		if !ok {
			return
		}
		data.SdnToOvnMigration = sdntoovnmigration
		if state, ok := migrationResponse.GetState(); ok {
			data.MigrationStateValue = state.Value()
		}
	}

	var retrievers []func()

	retrievers = append(
		retrievers,
		GetLimitedSupport,
		GetServiceLogs,
		GetJiraIssues,
		GetHandoverAnnouncements,
		GetSupportExceptions,
		GetPagerDutyAlerts,
		GetDynatraceDetails,
		GetBannedUser,
		GetMigrationInfo,
	)

	if o.queryOpts.Output == longOutputConfigValue {

		GetDescription := func() {
			defer wg.Done()
			defer utils.StartDelayTracker(o.queryOpts.Verbose, "Cluster Description").End()

			cmd := "ocm describe cluster " + o.clusterID
			output, err := exec.Command("bash", "-c", cmd).Output()
			if err != nil {
				fmt.Fprintln(os.Stderr, string(output))
				fmt.Fprintln(os.Stderr, err)
			}
			data.Description = string(output)
		}

		retrievers = append(
			retrievers,
			GetDescription,
		)
	}

	if o.queryOpts.FullScan {
		GetHistoricalPagerDutyAlerts := func() {
			pdwg.Wait()
			defer wg.Done()
			defer utils.StartDelayTracker(o.queryOpts.Verbose, "historical PagerDuty Alerts").End()
			data.HistoricalAlerts, err = pdProvider.GetHistoricalAlertsForCluster(data.pdServiceID)
			if err != nil {
				errors = append(errors, fmt.Errorf("error while getting historical PD Alert Data: %v", err))
			}
		}

		GetCloudTrailLogs := func() {
			defer wg.Done()
			defer utils.StartDelayTracker(o.queryOpts.Verbose, fmt.Sprintf("past %d pages of Cloudtrail data", o.queryOpts.Pages)).End()
			data.CloudtrailEvents, err = GetCloudTrailLogsForCluster(o.queryOpts.AWSProfile, o.clusterID, o.queryOpts.Pages)
			if err != nil {
				errors = append(errors, fmt.Errorf("error getting cloudtrail logs for cluster: %v", err))
			}
		}

		retrievers = append(
			retrievers,
			GetHistoricalPagerDutyAlerts,
			GetCloudTrailLogs,
		)
	}

	for _, retriever := range retrievers {
		wg.Add(1)
		go retriever()
	}

	wg.Wait()

	return data, errors
}

func GetCloudTrailLogsForCluster(awsProfile string, clusterID string, maxPages int) ([]*types.Event, error) {
	awsJumpClient, err := osdCloud.GenerateAWSClientForCluster(awsProfile, clusterID)
	if err != nil {
		return nil, err
	}

	var foundEvents []types.Event

	eventSearchInput := cloudtrail.LookupEventsInput{}

	for counter := 0; counter <= maxPages; counter++ {
		print(".")
		cloudTrailEvents, err := awsJumpClient.LookupEvents(&eventSearchInput)
		if err != nil {
			return nil, err
		}

		foundEvents = append(foundEvents, cloudTrailEvents.Events...)

		// for pagination
		eventSearchInput.NextToken = cloudTrailEvents.NextToken
		if cloudTrailEvents.NextToken == nil {
			break
		}
	}
	var filteredEvents []*types.Event
	for _, event := range foundEvents {
		if skippableEvent(*event.EventName) {
			continue
		}
		if event.Username != nil && strings.Contains(*event.Username, "RH-SRE-") {
			continue
		}
		filteredEvents = append(filteredEvents, &event)
	}

	return filteredEvents, nil
}

func printHistoricalPDAlertSummary(incidentCounters map[string][]*pagerduty.IncidentOccurrenceTracker, serviceIDs []string, sinceDays int, w io.Writer) {
	var name string = "PagerDuty Historical Alerts"
	fmt.Fprintln(w, delimiter+name)

	for _, serviceID := range serviceIDs {

		if len(incidentCounters[serviceID]) == 0 {
			fmt.Fprintln(w, "Service: https://redhat.pagerduty.com/service-directory/"+serviceID+": None")
			continue
		}

		fmt.Fprintln(w, "Service: https://redhat.pagerduty.com/service-directory/"+serviceID+":")
		table := printer.NewTablePrinter(w, 20, 1, 3, ' ')
		table.AddRow([]string{"Type", "Count", "Last Occurrence"})
		totalIncidents := 0
		for _, incident := range incidentCounters[serviceID] {
			table.AddRow([]string{incident.IncidentName, strconv.Itoa(incident.Count), incident.LastOccurrence})
			totalIncidents += incident.Count
		}

		// Add empty row for readability
		table.AddRow([]string{})
		if err := table.Flush(); err != nil {
			fmt.Fprintf(w, "Error printing %s: %v\n", name, err)
		}

		fmt.Fprintln(w, "\tTotal number of incidents [", totalIncidents, "] in [", sinceDays, "] days")
	}
}

func printJIRASupportExceptions(issues []jira.Issue, w io.Writer) {
	var name string = "Support Exceptions"
	fmt.Fprintln(w, delimiter+name)

	for _, i := range issues {
		fmt.Fprintf(w, "[%s](%s/%s): %+v [Status: %s]\n", i.Key, i.Fields.Type.Name, i.Fields.Priority.Name, i.Fields.Summary, i.Fields.Status.Name)
		fmt.Fprintf(w, "- Link: %s/browse/%s\n\n", JiraBaseURL, i.Key)
	}

	if len(issues) == 0 {
		fmt.Fprintln(w, "None")
	}
}

func printCloudTrailLogs(events []*types.Event, w io.Writer) {
	var name string = "Potentially interesting CloudTrail events"
	fmt.Fprintln(w, delimiter+name)

	if events == nil {
		fmt.Fprintln(w, "None")
		return
	}

	table := printer.NewTablePrinter(w, 20, 1, 3, ' ')
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
		fmt.Fprintf(w, "Error printing %s: %v\n", name, err)
	}
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

	for _, skipWord := range skippableList {
		if strings.Contains(eventName, skipWord) {
			return true
		}
	}
	return false
}

func printNetworkInfo(data *contextData, w io.Writer) {
	var name = "Network Info"
	fmt.Fprintln(w, delimiter+name)

	table := printer.NewTablePrinter(w, 20, 1, 3, ' ')
	table.AddRow([]string{"Network Type", data.NetworkType})
	table.AddRow([]string{"MachineCIDR", data.NetworkMachineCIDR})
	table.AddRow([]string{"ServiceCIDR", data.NetworkServiceCIDR})
	table.AddRow([]string{"Max Services", strconv.Itoa(data.NetworkMaxServices)})
	table.AddRow([]string{"PodCIDR", data.NetworkPodCIDR})
	table.AddRow([]string{"Host Prefix", strconv.Itoa(data.NetworkHostPrefix)})
	table.AddRow([]string{"Max Nodes (based on PodCIDR)", strconv.Itoa(data.NetworkMaxNodesFromPodCIDR)})
	table.AddRow([]string{"Max pods per node", strconv.Itoa(data.NetworkMaxPodsPerNode)})

	if err := table.Flush(); err != nil {
		fmt.Fprintf(w, "Error printing %s: %v\n", name, err)
	}
}

func printDynatraceResources(data *contextData, w io.Writer) {
	var name string = "Dynatrace Details"
	fmt.Fprintln(w, delimiter+name)

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

	table := printer.NewTablePrinter(w, 20, 1, 3, ' ')
	for _, link := range keys {
		url := strings.TrimSpace(links[link])
		if url == dynatrace.ErrUnsupportedCluster.Error() {
			fmt.Fprintln(w, dynatrace.ErrUnsupportedCluster.Error())
			break
		} else if url != "" {
			table.AddRow([]string{link, url})
		}
	}

	if err := table.Flush(); err != nil {
		fmt.Fprintf(w, "Error printing %s: %v\n", name, err)
	}
}

func printUserBannedStatus(data *contextData, w io.Writer) {
	var name string = "User Ban Details"
	fmt.Fprintln(w, "\n"+delimiter+name)
	if data.UserBanned {
		fmt.Fprintln(w, "User is banned")
		fmt.Fprintf(w, "Ban code = %v\n", data.BanCode)
		fmt.Fprintf(w, "Ban description = %v\n", data.BanDescription)
		if data.BanCode == BanCodeExportControlCompliance {
			fmt.Fprintln(w, "User banned due to export control compliance.\nPlease follow the steps detailed here: https://github.com/openshift/ops-sop/blob/master/v4/alerts/UpgradeConfigSyncFailureOver4HrSRE.md#user-banneddisabled-due-to-export-control-compliance .")
		}
	} else {
		fmt.Fprintln(w, "User is not banned")
	}
}

func (data *contextData) printClusterHeader(w io.Writer) {
	clusterHeader := fmt.Sprintf("%s -- %s", data.ClusterName, data.ClusterID)
	fmt.Fprintln(w, strings.Repeat("=", len(clusterHeader)))
	fmt.Fprintln(w, clusterHeader)
	fmt.Fprintln(w, strings.Repeat("=", len(clusterHeader)))
}

func printSDNtoOVNMigrationStatus(data *contextData, w io.Writer) {
	name := "SDN to OVN Migration Status"
	fmt.Fprintln(w, "\n"+delimiter+name)

	if data.SdnToOvnMigration != nil && data.MigrationStateValue == cmv1.ClusterMigrationStateValueInProgress {
		fmt.Fprintln(w, "SDN to OVN migration is in progress")
		return
	}

	fmt.Fprintln(w, "No active SDN to OVN migrations")
}
