package dynatrace

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	dryRun        bool
	tail          int
	since         int
	contains      string
	sortOrder     string
	namespaceList []string
	nodeList      []string
	podList       []string
	containerList []string
	statusList    []string
)

func NewCmdLogs() *cobra.Command {
	logsCmd := &cobra.Command{
		Use:   "logs <cluster-id>",
		Short: "Fetch logs from Dynatrace",
		Long: `Fetch logs from Dynatrace and display the logs like oc logs.

  This command also prints the Dynatrace URL and the corresponding DQL in the output.`,
		Example: `
  # Get the logs of HCP cluster hcp-cluster-id-123.
  # Specify to get the logs of the pod alertmanager-main-0 in namespace openshift-monitoring
  osdctl cluster dynatrace logs hcp-cluster-id-123 --namespace openshift-monitoring --pod alertmanager-main-0`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			err := main(args[0])
			if err != nil {
				cmdutil.CheckErr(err)
			}
		},
	}

	logsCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Only builds the query without fetching any logs from the tenant")
	logsCmd.Flags().IntVar(&tail, "tail", 100, "Last 'n' logs to fetch (defaults to 100)")
	logsCmd.Flags().IntVar(&since, "since", 1, "Number of hours (integer) since which to search (defaults to 1 hour)")
	logsCmd.Flags().StringVar(&contains, "contains", "", "Include logs which contain a phrase")
	logsCmd.Flags().StringVar(&sortOrder, "sort", "desc", "Sort the results by timestamp in either ascending or descending order. Accepted values are 'asc' and 'desc'")
	logsCmd.Flags().StringSliceVar(&namespaceList, "namespace", []string{}, "Namespace(s) (comma-separated)")
	logsCmd.Flags().StringSliceVar(&nodeList, "node", []string{}, "Node name(s) (comma-separated)")
	logsCmd.Flags().StringSliceVar(&podList, "pod", []string{}, "Pod name(s) (comma-separated)")
	logsCmd.Flags().StringSliceVar(&containerList, "container", []string{}, "Container name(s) (comma-separated)")
	logsCmd.Flags().StringSliceVar(&statusList, "status", []string{}, "Status(Info/Warn/Error) (comma-separated)")

	return logsCmd
}

func GetLinkToWebConsole(dtURL string, since int, finalQuery string) (string, error) {
	SearchQuery := map[string]interface{}{
		"version": "0",
		"data": map[string]interface{}{
			"tableConfig": map[string]interface{}{
				"visibleColumns": []string{"timestamp", "status", "content"},
				"columnAttributes": map[string]interface{}{
					"columnWidths": map[string]interface{}{},
					"lineWraps": map[string]interface{}{
						"timestamp": true,
						"status":    true,
						"content":   true,
					},
					"tableLineWrap": true,
				},
				"columnOrder": []string{"timestamp", "status", "content"},
			},
			"queryConfig": map[string]interface{}{
				"query":     finalQuery,
				"timeframe": map[string]interface{}{"from": fmt.Sprintf("now()-%vh", since), "to": "now()"},
				"filter": map[string]interface{}{
					"datatype": "logs",
					"filters":  map[string]interface{}{},
					"sort": map[string]interface{}{
						"field":     "timestamp",
						"direction": "desc",
					},
				},
				"showDqlEditor": true,
			},
		},
	}
	mStr, err := json.Marshal(SearchQuery)
	if err != nil {
		return "", fmt.Errorf("failed to create JSON for sharable URL: %v", err)
	}
	return fmt.Sprintf("%sui/apps/dynatrace.logs/?gtf=-%dh&gf=all&sortDirection=desc&advancedQueryMode=true&isDefaultQuery=false&visualizationType=table#%s\n\n", dtURL, since, url.PathEscape(string(mStr))), nil
}

func main(clusterID string) error {
	var hcpCluster HCPCluster
	if since <= 0 {
		return fmt.Errorf("invalid time duration")
	}
	hcpCluster, err := FetchClusterDetails(clusterID)
	if err != nil {
		return fmt.Errorf("failed to acquire cluster details %v", err)
	}

	query, err := GetQuery(hcpCluster)
	if err != nil {
		return fmt.Errorf("failed to build query for Dynatrace %v", err)
	}

	fmt.Println(query.Build())

	url, err := GetLinkToWebConsole(hcpCluster.DynatraceURL, since, query.finalQuery)

	if err != nil {
		return fmt.Errorf("failed to get url: %v:", err)
	}

	fmt.Println("\nLink to Web Console - \n", url)

	if dryRun {
		return nil
	}

	accessToken, err := getAccessToken()
	if err != nil {
		return fmt.Errorf("failed to acquire access token %v", err)
	}

	requestToken, err := getDTQueryExecution(hcpCluster.DynatraceURL, accessToken, query.finalQuery)
	if err != nil {
		return fmt.Errorf("failed to get  vault token %v", err)
	}
	err = getLogs(hcpCluster.DynatraceURL, accessToken, requestToken, nil)
	if err != nil {
		return fmt.Errorf("failed to get logs %v", err)
	}

	return nil
}

func GetQuery(hcpCluster HCPCluster) (query DTQuery, error error) {
	q := DTQuery{}
	q.InitLogs(since).Cluster(hcpCluster.managementClusterName)

	if hcpCluster.hcpNamespace != "" {
		namespaceList = append(namespaceList, hcpCluster.hcpNamespace)
	}

	if len(namespaceList) > 0 {
		q.Namespaces(namespaceList)
	}

	if len(nodeList) > 0 {
		q.Nodes(nodeList)
	}

	if len(podList) > 0 {
		q.Pods(podList)
	}

	if len(containerList) > 0 {
		q.Containers(containerList)
	}

	if len(statusList) > 0 {
		q.Status(statusList)
	}

	if contains != "" {
		q.ContainsPhrase(contains)
	}

	if sortOrder != "" {
		q, err := q.Sort(sortOrder)
		if err != nil {
			return *q, err
		}
	}

	if tail > 0 {
		q.Limit(tail)
	}

	return q, nil
}
