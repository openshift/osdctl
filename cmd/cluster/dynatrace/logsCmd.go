package dynatrace

import (
	"encoding/base64"
	"fmt"

	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	dryRun        bool
	hcp           bool
	tail          int
	since         int
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
		Use:               "logs <cluster-id>",
		Short:             "Fetch logs from Dynatrace",
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
	logsCmd.Flags().BoolVar(&hcp, "hcp", false, "Set true to Include the HCP Namespace")
	logsCmd.Flags().StringSliceVar(&namespaceList, "namespace", []string{}, "Namespace(s) (comma-separated)")
	logsCmd.Flags().StringSliceVar(&nodeList, "node", []string{}, "Node name(s) (comma-separated)")
	logsCmd.Flags().StringSliceVar(&podList, "pod", []string{}, "Pod name(s) (comma-separated)")
	logsCmd.Flags().StringSliceVar(&containerList, "container", []string{}, "Container name(s) (comma-separated)")
	logsCmd.Flags().StringSliceVar(&statusList, "status", []string{}, "Status(Info/Warn/Error) (comma-separated)")

	return logsCmd
}

func getLinkToWebConsole(dtURL string, since int, base64Url string) string {
	return fmt.Sprintf("\nLink to Web Console - \n%sui/apps/dynatrace.classic.logs.events/ui/logs-events?gtf=-%dh&gf=all&sortDirection=desc&advancedQueryMode=true&isDefaultQuery=false&visualizationType=table#%s\n\n", dtURL, since, base64Url)
}

func main(clusterID string) error {
	if since <= 0 {
		return fmt.Errorf("invalid time duration")
	}

	clusterInternalID, mgmtClusterName, DTURL, err := fetchClusterDetails(clusterID)
	if err != nil {
		return fmt.Errorf("failed to acquire cluster details %v", err)
	}

	accessToken, err := getAccessToken()
	if err != nil {
		return fmt.Errorf("failed to acquire access token %v", err)
	}

	query, err := getQuery(clusterInternalID, mgmtClusterName)
	if err != nil {
		return fmt.Errorf("failed to build query for Dynatrace %v", err)
	}

	fmt.Println(query.Build())
	fmt.Println(getLinkToWebConsole(DTURL, since, base64.StdEncoding.EncodeToString([]byte(query.finalQuery))))

	if dryRun {
		return nil
	}

	requestToken, err := getRequestToken(query.finalQuery, DTURL, accessToken)
	if err != nil {
		return fmt.Errorf("failed to acquire request token %v", err)
	}

	err = getLogs(DTURL, accessToken, requestToken, nil)
	if err != nil {
		return fmt.Errorf("failed to get logs %v", err)
	}

	return nil
}

func getQuery(clusterID string, mgmtClusterName string) (query DTQuery, error error) {
	q := DTQuery{}
	q.InitLogs(since).Cluster(mgmtClusterName)

	if len(namespaceList) > 0 || hcp {
		if hcp {
			managementClusterInternalID, _, _, err := fetchClusterDetails(mgmtClusterName)
			if err != nil {
				return q, err
			}
			clientset, err := getClientsetFromClusterID(managementClusterInternalID)
			if err != nil {
				return q, err
			}
			hcpNS, err := GetHCPNamespaceFromInternalID(clientset, clusterID)
			if err != nil {
				return q, err
			}
			namespaceList = append(namespaceList, hcpNS)
		}
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
