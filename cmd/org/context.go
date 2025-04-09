package org

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/andygrunwald/go-jira"
	accountsv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
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

const ServiceLogDaysSince = 30

var contextCmd = ContextCmd(NewDefaultContextFetcher())

type ContextFetcher interface {
	FetchContext(orgID string, output io.Writer) ([]ClusterInfo, error)
}

type DefaultContextFetcher struct {
	CreateOCMClient     func() (*sdk.Connection, error)
	SearchSubscriptions func(orgID string, status string, managedOnly bool) ([]*accountsv1.Subscription, error)
	GetCluster          func(*sdk.Connection, string) (*cmv1.Cluster, error)
	GetLimitedSupport   func(*sdk.Connection, string) ([]*cmv1.LimitedSupportReason, error)
	GetServiceLogs      func(string, time.Time, bool, bool) ([]*v1.LogEntry, error)
	GetJiraIssues       func(clusterID, externalID, filter string) ([]jira.Issue, error)
	NewPDClient         func(baseDomain string) (PDClient, error)
}

type PDClient interface {
	GetPDServiceIDs() ([]string, error)
	GetFiringAlertsForCluster([]string) (map[string][]pd.Incident, error)
}

func NewDefaultContextFetcher() *DefaultContextFetcher {
	return &DefaultContextFetcher{
		CreateOCMClient:     utils.CreateConnection,
		SearchSubscriptions: SearchAllSubscriptionsByOrg,
		GetCluster:          utils.GetCluster,
		GetLimitedSupport:   utils.GetClusterLimitedSupportReasons,
		GetServiceLogs:      servicelog.GetServiceLogsSince,
		GetJiraIssues:       utils.GetJiraIssuesForCluster,
		NewPDClient: func(baseDomain string) (PDClient, error) {
			return pdProvider.NewClient().
				WithBaseDomain(baseDomain).
				WithUserToken(viper.GetString(pdProvider.PagerDutyUserTokenConfigKey)).
				WithOauthToken(viper.GetString(pdProvider.PagerDutyOauthTokenConfigKey)).
				Init()
		},
	}
}

func ContextCmd(fetcher ContextFetcher) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "context orgId",
		Short:   "fetches information about the given organization",
		Long:    `Fetches information about the given organization. This data is presented as a table where each row includes the name, version, ID, cloud provider, and plan for the cluster. Rows will also include the number of recent service logs, active PD Alerts, Jira Issues, and limited support status for that specific cluster.`,
		Example: `# Get context data for a cluster
osdctl org context 1a2B3c4DefghIjkLMNOpQrSTUV5

# Get context data in JSON format
osdctl org context 1a2B3c4DefghIjkLMNOpQrSTUV5 -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat, err := cmd.Flags().GetString("output")
			if err != nil {
				return fmt.Errorf("error reading flag 'output': %w", err)
			}
			if outputFormat != "" && outputFormat != "json" {
				return errors.New("unsupported output format, only 'json' is accepted")
			}

			// Progress goes to stderr, JSON to stdout
			progressWriter := os.Stderr

			clusterInfos, err := fetcher.FetchContext(args[0], progressWriter)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error fetching org context: %v\n", err)
			}
			if len(clusterInfos) == 0 {
				fmt.Println("Org has no clusters")
				return nil
			}

			if outputFormat == "json" {
				return printContextJson(os.Stdout, clusterInfos)
			}
			return printContext(clusterInfos)
		},
	}
	cmd.Flags().StringP("output", "o", "", "output format for the results. only supported value currently is 'json'")
	return cmd
}

func (f *DefaultContextFetcher) FetchContext(orgID string, output io.Writer) ([]ClusterInfo, error) {
	subscriptions, err := f.SearchSubscriptions(orgID, StatusActive, true)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cluster subscriptions for org with ID %s: %w", orgID, err)
	}
	total := len(subscriptions)
	if total == 0 {
		return nil, nil
	}

	ocmClient, err := f.CreateOCMClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create OCM client: %w", err)
	}
	defer ocmClient.Close()

	var (
		results []ClusterInfo
		mutex   sync.Mutex
		count   int
	)
	eg, _ := errgroup.WithContext(context.Background())

	fmt.Fprintf(output, "Fetching data for %v clusters in org %v...\n", total, orgID)
	fmt.Fprintf(output, "Fetched data for 0 of %v clusters...\n", total)

	for _, sub := range subscriptions {
		sub := sub
		eg.Go(func() error {
			cluster, err := f.GetCluster(ocmClient, sub.ClusterID())
			if err != nil {
				return fmt.Errorf("failed to get cluster %s: %w", sub.ClusterID(), err)
			}

			ci := ClusterInfo{
				Name:          cluster.Name(),
				Version:       cluster.Version().RawID(),
				ID:            cluster.ID(),
				CloudProvider: sub.CloudProviderID(),
				Plan:          sub.Plan().ID(),
			}
			if metrics, ok := sub.GetMetrics(); ok {
				ci.NodeCount = metrics[0].Nodes().Total()
			}

			dataEg, _ := errgroup.WithContext(context.Background())
			// Limited support reasons
			dataEg.Go(func() error {
				ci.LimitedSupportReasons, err = f.GetLimitedSupport(ocmClient, ci.ID)
				if err != nil {
					return fmt.Errorf("failed to fetch limited support reasons for cluster %v: %w", ci.ID, err)
				}
				return nil
			})
			// Service logs
			dataEg.Go(func() error {
				ci.ServiceLogs, err = f.GetServiceLogs(ci.ID, time.Now().AddDate(0, 0, -ServiceLogDaysSince), false, false)
				if err != nil {
					return fmt.Errorf("failed to fetch service logs for cluster %v: %w", ci.ID, err)
				}
				return nil
			})
			// Jira issues
			dataEg.Go(func() error {
				ci.JiraIssues, err = f.GetJiraIssues(ci.ID, cluster.ExternalID(), "")
				if err != nil {
					return fmt.Errorf("failed to fetch Jira issues for cluster %v: %v", ci.ID, err)
				}
				return nil
			})
			// PagerDuty alerts
			dataEg.Go(func() error {
				pdClient, err := f.NewPDClient(cluster.DNS().BaseDomain())
				if err != nil {
					return fmt.Errorf("failed to build PD client")
				}
				serviceIDs, err := pdClient.GetPDServiceIDs()
				if err != nil {
					return fmt.Errorf("failed to get PD ServiceID for cluster %v: %v", ci.ID, err)
				}
				ci.PdAlerts, err = pdClient.GetFiringAlertsForCluster(serviceIDs)
				if err != nil {
					return fmt.Errorf("failed to get PD Alerts for cluster %v: %v", ci.ID, err)
				}
				return nil
			})
			if err := dataEg.Wait(); err != nil {
				return err
			}

			mutex.Lock()
			count++
			fmt.Fprintf(output, "\033[1A\033[K")
			fmt.Fprintf(output, "Fetched data for %v of %v clusters...\n", count, total)
			results = append(results, ci)
			mutex.Unlock()

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return results, fmt.Errorf("failed to get context data: %w", err)
	}
	return results, nil
}

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

func printContextJson(w io.Writer, clusterInfos []ClusterInfo) error {
	views := make([]clusterInfoView, 0, len(clusterInfos))
	for _, ci := range clusterInfos {
		plan := ci.Plan
		if plan == "MOA" {
			plan = "ROSA"
		}
		if plan == "MOA-HostedControlPlane" {
			plan = "HCP"
		}
		views = append(views, clusterInfoView{
			DisplayName: ci.Name,
			ClusterId:   ci.ID,
			Version:     ci.Version,
			Status:      getSupportStatusDisplayText(ci.LimitedSupportReasons),
			Provider:    ci.CloudProvider,
			Plan:        plan,
			NodeCount:   ci.NodeCount,
			RecentSLs:   len(ci.ServiceLogs),
			ActivePDs:   len(ci.PdAlerts),
			OHSS:        len(ci.JiraIssues),
		})
	}
	bytes, err := json.MarshalIndent(views, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal json response: %w", err)
	}
	_, err = fmt.Fprintln(w, string(bytes))
	return err
}

func printContext(clusterInfos []ClusterInfo) error {
	table := printer.NewTablePrinter(os.Stdout, 0, 1, 3, ' ')
	table.AddRow([]string{"DISPLAY NAME", "CLUSTER ID", "VERSION", "STATUS", "PROVIDER", "PLAN", "NODE COUNT", "RECENT SLs", "ACTIVE PDs", "OHSS TICKETS"})
	for _, ci := range clusterInfos {
		recentSLs := len(ci.ServiceLogs)
		activePDs := len(ci.PdAlerts)
		ohss := len(ci.JiraIssues)
		table.AddRow([]string{
			ci.Name,
			ci.ID,
			ci.Version,
			getSupportStatusDisplayText(ci.LimitedSupportReasons),
			ci.CloudProvider,
			getPlanDisplayText(ci.Plan),
			fmt.Sprintf("%v", ci.NodeCount),
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

func getSupportStatusDisplayText(reasons []*cmv1.LimitedSupportReason) string {
	if len(reasons) > 0 {
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
