package dynatrace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewCmdHCPLogs() *cobra.Command {
	hcpLogsCmd := &cobra.Command{
		Use:               "hcp-logs <cluster-id>",
		Aliases:           []string{"hl"},
		Short:             "Gather all Pod logs from HCP",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			err := hcpLogs(args[0])
			if err != nil {
				cmdutil.CheckErr(err)
			}
		},
	}

	return hcpLogsCmd
}

func hcpLogs(clusterID string) (error error) {
	accessToken, err := getAccessToken()
	if err != nil {
		return fmt.Errorf("failed to acquire access token %v", err)
	}

	clusterInternalID, managementClusterName, DTURL, err := fetchClusterDetails(clusterID)
	if err != nil {
		return err
	}
	managementClusterInternalID, _, _, err := fetchClusterDetails(managementClusterName)
	if err != nil {
		return err
	}

	connection, err := getConnection()
	if err != nil {
		return err
	}
	clientset, err := getClientsetFromClusterID(connection, managementClusterInternalID)
	if err != nil {
		return err
	}

	hcpNS, err := GetHCPNamespaceFromInternalID(clientset, clusterInternalID)
	if err != nil {
		return err
	}

	fmt.Println(fmt.Sprintf("Using HCP Namespace %v", hcpNS))

	logsDir, err := setupTempLogDir(hcpNS)
	if err != nil {
		return err
	}

	pods, err := getPodsForNamespace(clientset, hcpNS)
	if err != nil {
		return err
	}

	err = dumpPodLogs(pods, logsDir, hcpNS, managementClusterName, DTURL, accessToken)
	if err != nil {
		return err
	}

	return nil
}

func dumpPodLogs(pods []string, logsDir string, hcpNS string, managementClusterName string, DTURL string, accessToken string) error {
	totalPods := len(pods)
	for k, p := range pods {
		fmt.Println(fmt.Sprintf("[%d/%d] Pod logs for %s", k+1, totalPods, p))
		podLogsQuery, err := getPodQuery(p, hcpNS, 2, managementClusterName)
		if err != nil {
			return err
		}
		podLogsQuery.Build()

		requestToken, err := getRequestToken(podLogsQuery.finalQuery, DTURL, accessToken)
		if err != nil {
			return fmt.Errorf("failed to acquire request token %v", err)
		}

		podLogFilePath, err := addPodDir(logsDir, p)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(podLogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0655)
		if err != nil {
			return err
		}

		err = getLogs(DTURL, accessToken, requestToken, f)
		defer f.Close()
		if err != nil {
			return fmt.Errorf("failed to get logs %v", err)
		}
	}

	return nil
}

func setupTempLogDir(dirName string) (logsDir string, error error) {
	dirPath := filepath.Join(".", fmt.Sprintf("hcp-logs-%s", dirName))
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to setup logs directory %v", err)
	}

	return dirPath, nil
}

func addPodDir(logsDir string, podName string) (path string, error error) {
	podPath := filepath.Join(logsDir, podName)
	err := os.MkdirAll(podPath, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to setup pod directory %v", err)
	}
	podLogPath := filepath.Join(podPath, "logs")
	_, err = os.Create(podLogPath)
	return podLogPath, nil
}

func getPodQuery(pod string, namespace string, since int, srcCluster string) (query DTQuery, error error) {
	q := DTQuery{}
	q.Init(since).Cluster(srcCluster)

	if namespace != "" {
		q.Namespaces([]string{namespace})
	}

	if pod != "" {
		q.Pods([]string{pod})
	}

	return q, nil
}

func getPodsForNamespace(clientset *kubernetes.Clientset, namespace string) (pl []string, error error) {
	// Getting pod objects for non-running state pod
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace '%s'", namespace)
	}

	podList := []string{}
	for _, pod := range pods.Items {
		podList = append(podList, pod.Name)
	}

	return podList, nil
}
