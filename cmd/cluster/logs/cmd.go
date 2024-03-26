package logs

import (
	"encoding/base64"
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

const (
	DynatraceTenantKeyLabel    string = "sre-capabilities.dtp.tenant"
	HypershiftClusterTypeLabel string = "ext-hypershift.openshift.io/cluster-type"
	authURL                    string = "https://sso.dynatrace.com/sso/oauth2/token"
	vaultPath                  string = "osd-sre/dynatrace/sd-sre-platform-oauth-client-grail"
	vaultAddr                  string = "https://vault.devshift.net"
)

var (
	dryRun        bool
	hcp           bool
	tail          int
	hours         int
	contains      string
	cluster       string
	sortOrder     string
	namespaceList []string
	nodeList      []string
	podList       []string
	containerList []string
	statusList    []string
)

func NewCmdLogs() *cobra.Command {
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Fetch logs from Dynatrace",
		Args:  cobra.NoArgs,
		RunE:  Run,
	}

	logsCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Only builds the query without fetching any logs from the tenant")
	logsCmd.Flags().IntVar(&tail, "tail", 10, "Last 'n' logs to fetch")
	logsCmd.Flags().IntVar(&hours, "hours", 1, "Number of hours back to search")
	logsCmd.Flags().StringVar(&contains, "contains", "", "Include logs which contain a phrase")
	logsCmd.Flags().StringVar(&cluster, "cluster", "", "Cluster name")
	logsCmd.Flags().StringVar(&sortOrder, "sort", "", "Sort the results by timestamp in either ascending or descending order")
	logsCmd.Flags().BoolVar(&hcp, "hcp", false, "Set true to Include the HCP Namespace")
	logsCmd.Flags().StringSliceVar(&namespaceList, "namespace", []string{}, "Namespace(s) (comma-separated)")
	logsCmd.Flags().StringSliceVar(&nodeList, "node", []string{}, "Node name(s) (comma-separated)")
	logsCmd.Flags().StringSliceVar(&podList, "pod", []string{}, "Pod name(s) (comma-separated)")
	logsCmd.Flags().StringSliceVar(&containerList, "container", []string{}, "Container name(s) (comma-separated)")
	logsCmd.Flags().StringSliceVar(&statusList, "status", []string{}, "Status(Info/Warn/Error) (comma-separated)")

	return logsCmd
}

func errorExit(err error) {
	fmt.Println(err)
	os.Exit(1)
}

func getBase64Url(query string) string {
	return base64.StdEncoding.EncodeToString([]byte(query))
}

func getLinkToWebConsole(dtUrl string, hours int, base64Url string) string {
	return fmt.Sprintf("\nLink to Web Console - \n%sui/apps/dynatrace.classic.logs.events/ui/logs-events?gtf=-%dh&gf=all&sortDirection=desc&advancedQueryMode=true&isDefaultQuery=false&visualizationType=table#%s\n\n", dtUrl, hours, base64Url)
}

func Run(cmd *cobra.Command, args []string) error {
	if cluster == "" {
		return fmt.Errorf("Cluster name cannot be left blank")
	}

	if hours <= 0 {
		return fmt.Errorf("Invalid time duration")
	}

	clusterInternalID, mgmtClusterName, DTURL := fetchDetails(cluster)

	query := getQuery(clusterInternalID, mgmtClusterName)

	fmt.Println(query.Build())
	fmt.Println(getLinkToWebConsole(DTURL, hours, getBase64Url(query.finalQuery)))

	if dryRun {
		return nil
	}

	accessToken, err := getAccessToken()
	if err != nil {
		errorExit(err)
	}

	requestToken, err := getRequestToken(query.finalQuery, DTURL, accessToken)
	if err != nil {
		errorExit(err)
	}

	err = getLogs(DTURL, accessToken, requestToken)
	if err != nil {
		errorExit(err)
	}

	return nil
}

func getQuery(clusterID string, mgmtClusterName string) DTQuery {
	query := DTQuery{}
	query.Init(hours).Cluster(mgmtClusterName)

	if len(namespaceList) > 0 || hcp {
		query.Namespaces(namespaceList, clusterID, hcp)
	}

	if len(nodeList) > 0 {
		query.Nodes(nodeList)
	}

	if len(podList) > 0 {
		query.Pods(podList)
	}

	if len(containerList) > 0 {
		query.Containers(containerList)
	}

	if len(statusList) > 0 {
		query.Status(statusList)
	}

	if contains != "" {
		query.ContainsPhrase(contains)
	}

	if sortOrder != "" {
		query.Sort(sortOrder)
	}

	if tail > 0 {
		query.Limit(tail)
	}

	return query
}
