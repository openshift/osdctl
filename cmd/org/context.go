package org

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/andygrunwald/go-jira"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	v1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/printer"
	pdProvider "github.com/openshift/osdctl/pkg/provider/pagerduty"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

const (
	ServiceLogDaysSince = 30
)

type ClusterInfo struct {
	Name                  string
	Version               string
	ID                    string
	CloudProvider         string
	Plan                  string
	NodeCount             float64
	ServiceLogs           []*v1.LogEntry
	PdAlerts              map[string][]pd.Incident
	JiraIssues            []jira.Issue
	LimitedSupportReasons []*cmv1.LimitedSupportReason
}

type clusterInfoView struct {
	DisplayName string  `json:"displayName"`
	ClusterId   string  `json:"clusterId"`
	Version     string  `json:"version"`
	Status      string  `json:"status"`
	Provider    string  `json:"provider"`
	Plan        string  `json:"plan"`
	NodeCount   float64 `json:"nodeCount"`
	RecentSLs   int     `json:"recentSLs"`
	ActivePDs   int     `json:"activePDs"`
	OHSS        int     `json:"ohssTickets"`
}

var contextCmd = &cobra.Command{
	Use:   "context orgId",
	Short: "fetches information about the given organization",
	Long: `Fetches information about the given organization. This data is presented as a table where each row includes the name, version, ID, cloud provider, and plan for the cluster.
Rows will also include the number of recent service logs, active PD Alerts, Jira Issues, and limited support status for that specific cluster.`,
	Example: `# Get context data for a cluster
osdctl org context 1a2B3c4DefghIjkLMNOpQrSTUV5

#Get context data for cluster in json format
osdctl org context 1a2B3c4DefghIjkLMNOpQrSTUV5 -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outputFormat, err := cmd.Flags().GetString("output")
		if err != nil {
			return fmt.Errorf("error reading flag 'json': %w", err)
		}
		if outputFormat != "" && outputFormat != "json" {
			return errors.New("unsupported output format, only 'json' is accepted")
		}

		clusterInfos, err := Context(args[0])
		if err != nil {
			// report error, but don't return: we should make a best-effort attempt at printing whatever we did retrieve
			fmt.Fprintf(os.Stderr, "error fetching org context: %v\n", err)
		}
		if len(clusterInfos) == 0 {
			fmt.Println("Org has no clusters")
			return nil
		}

		if outputFormat == "json" {
			return printContextJson(clusterInfos)
		}

		return printContext(clusterInfos)
	},
}

func init() {
	contextCmd.Flags().StringP("output", "o", "", "output format for the results. only supported value currently is 'json'")
}

func printContextJson(clusterInfos []ClusterInfo) error {
	clusterInfoViews := make([]clusterInfoView, 0, len(clusterInfos))
	for _, clusterInfo := range clusterInfos {
		plan := clusterInfo.Plan
		if plan == "MOA" {
			plan = "ROSA"
		}
		if plan == "MOA-HostedControlPlane" {
			plan = "HCP"
		}

		view := clusterInfoView{
			DisplayName: clusterInfo.Name,
			ClusterId:   clusterInfo.ID,
			Version:     clusterInfo.Version,
			Status:      getSupportStatusDisplayText(clusterInfo.LimitedSupportReasons),
			Provider:    clusterInfo.CloudProvider,
			Plan:        plan,
			NodeCount:   clusterInfo.NodeCount,
			RecentSLs:   len(clusterInfo.ServiceLogs),
			ActivePDs:   len(clusterInfo.PdAlerts),
			OHSS:        len(clusterInfo.JiraIssues),
		}

		clusterInfoViews = append(clusterInfoViews, view)
	}

	bytes, err := json.MarshalIndent(clusterInfoViews, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal json response: %w", err)
	}
	fmt.Println(string(bytes))

	return nil
}

func printContext(clusterInfos []ClusterInfo) error {
	table := printer.NewTablePrinter(os.Stdout, 0, 1, 3, ' ')
	table.AddRow([]string{"DISPLAY NAME", "CLUSTER ID", "VERSION", "STATUS", "PROVIDER", "PLAN", "NODE COUNT", "RECENT SLs", "ACTIVE PDs", "OHSS TICKETS"})

	for _, clusterInfo := range clusterInfos {
		recentSLs := len(clusterInfo.ServiceLogs)
		activePDs := len(clusterInfo.PdAlerts)
		ohss := len(clusterInfo.JiraIssues)
		table.AddRow([]string{
			clusterInfo.Name,
			clusterInfo.ID,
			clusterInfo.Version,
			getSupportStatusDisplayText(clusterInfo.LimitedSupportReasons),
			clusterInfo.CloudProvider,
			getPlanDisplayText(clusterInfo.Plan),
			fmt.Sprintf("%v", clusterInfo.NodeCount),
			strconv.Itoa(recentSLs),
			strconv.Itoa(activePDs),
			strconv.Itoa(ohss),
		})
	}

	table.AddRow([]string{})
	if err := table.Flush(); err != nil {
		return fmt.Errorf("error writing data to console: %w", err)
	}
	return nil
}

func getSupportStatusDisplayText(limitedSupportReasons []*cmv1.LimitedSupportReason) string {
	if len(limitedSupportReasons) > 0 {
		return "Limited Support"
	}
	return "Fully Supported"
}

func getPlanDisplayText(plan string) string {
	if plan == "MOA" {
		return "ROSA"
	}
	if plan == "MOA-HostedControlPlane" {
		return "HCP"
	}
	return plan
}

func Context(orgId string) ([]ClusterInfo, error) {
	clusterSubscriptions, err := SearchAllSubscriptionsByOrg(orgId, StatusActive, true)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cluster subscriptions for org with ID %s: %w", orgId, err)
	}

	clusterSubscriptionsCount := len(clusterSubscriptions)
	if clusterSubscriptionsCount == 0 {
		return nil, nil
	}

	// cluster info
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to create OCM client: %w", err)
	}
	defer ocmClient.Close()

	var orgClustersInfo []ClusterInfo

	eg, ctx := errgroup.WithContext(context.Background())
	var mutex sync.Mutex
	count := 0

	// Print these to stderr so the actual results can be piped to different parsers without worrying about these lines
	_, _ = fmt.Fprintf(os.Stderr, "Fetching data for %v clusters in org %v...\n", clusterSubscriptionsCount, orgId)
	_, _ = fmt.Fprintf(os.Stderr, "Fetched data for 0 of %v clusters...\n", clusterSubscriptionsCount)
	for _, subscription := range clusterSubscriptions {
		sub := subscription
		eg.Go(func() error {
			defer ctx.Done()
			clusterId := sub.ClusterID()
			cluster, getClusterErr := utils.GetCluster(ocmClient, clusterId)
			if getClusterErr != nil {
				return fmt.Errorf("failed to get cluster %s: %w", clusterId, getClusterErr)
			}

			clusterInfo := ClusterInfo{
				Name:          cluster.Name(),
				Version:       cluster.Version().RawID(),
				ID:            cluster.ID(),
				CloudProvider: sub.CloudProviderID(),
				Plan:          sub.Plan().ID(),
			}

			if metrics, ok := sub.GetMetrics(); ok {
				clusterInfo.NodeCount = metrics[0].Nodes().Total()
			}

			dataErrs, dataCtx := errgroup.WithContext(context.Background())
			dataErrs.Go(func() error {
				defer dataCtx.Done()
				ci := &clusterInfo
				return addLimitedSupportReasons(ci, ocmClient)
			})

			dataErrs.Go(func() error {
				defer dataCtx.Done()
				ci := &clusterInfo
				return addServiceLogs(ci)
			})

			dataErrs.Go(func() error {
				defer dataCtx.Done()
				ci := &clusterInfo
				externalId := cluster.ExternalID()
				return addJiraIssues(ci, externalId)
			})

			dataErrs.Go(func() error {
				defer dataCtx.Done()
				ci := &clusterInfo
				baseDomain := cluster.DNS().BaseDomain()
				return addPDAlerts(ci, baseDomain)
			})

			if errs := dataErrs.Wait(); errs != nil {
				return errs
			}

			mutex.Lock()
			_, _ = fmt.Fprintf(os.Stderr, "\033[1A\033[K")
			count++
			_, _ = fmt.Fprintf(os.Stderr, "Fetched data for %v of %v clusters...\n", count, clusterSubscriptionsCount)
			orgClustersInfo = append(orgClustersInfo, clusterInfo)
			mutex.Unlock()

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return orgClustersInfo, fmt.Errorf("failed to get context data: %w", err)
	}
	return orgClustersInfo, nil
}

func addLimitedSupportReasons(clusterInfo *ClusterInfo, ocmClient *sdk.Connection) error {
	var limitedSupportReasonsErr error
	clusterInfo.LimitedSupportReasons, limitedSupportReasonsErr = utils.GetClusterLimitedSupportReasons(ocmClient, clusterInfo.ID)
	if limitedSupportReasonsErr != nil {
		return fmt.Errorf("failed to fetch limited support reasons for cluster %v: %w", clusterInfo.ID, limitedSupportReasonsErr)
	}
	return nil
}

func addServiceLogs(clusterInfo *ClusterInfo) error {
	var err error
	timeToCheckSvcLogs := time.Now().AddDate(0, 0, -ServiceLogDaysSince)
	clusterInfo.ServiceLogs, err = servicelog.GetServiceLogsSince(clusterInfo.ID, timeToCheckSvcLogs, false, false)
	if err != nil {
		return fmt.Errorf("failed to fetch service logs for cluster %v: %w", clusterInfo.ID, err)
	}
	return nil
}

func addJiraIssues(clusterInfo *ClusterInfo, externalId string) error {
	var jiraIssuesErr error
	clusterInfo.JiraIssues, jiraIssuesErr = utils.GetJiraIssuesForCluster(clusterInfo.ID, externalId, "")
	if jiraIssuesErr != nil {
		return fmt.Errorf("failed to fetch Jira issues for cluster %v: %v", clusterInfo.ID, jiraIssuesErr)
	}
	return nil
}

func addPDAlerts(clusterInfo *ClusterInfo, baseDomain string) error {
	pdClient, err := pdProvider.NewClient().
		WithBaseDomain(baseDomain).
		WithUserToken(viper.GetString(pdProvider.PagerDutyUserTokenConfigKey)).
		WithOauthToken(viper.GetString(pdProvider.PagerDutyOauthTokenConfigKey)).
		Init()
	if err != nil {
		return fmt.Errorf("failed to build PD client")
	}
	pdServiceIds, pdServiceIdsErr := pdClient.GetPDServiceIDs()
	if pdServiceIdsErr != nil {
		return fmt.Errorf("failed to get PD ServiceID for cluster %v: %v", clusterInfo.ID, pdServiceIdsErr)
	} else {
		var pdAlertsErr error
		clusterInfo.PdAlerts, pdAlertsErr = pdClient.GetFiringAlertsForCluster(pdServiceIds)
		if pdAlertsErr != nil {
			return fmt.Errorf("failed to get PD Alerts for cluster %v: %v", clusterInfo.ID, pdAlertsErr)
		}
	}
	return nil
}
