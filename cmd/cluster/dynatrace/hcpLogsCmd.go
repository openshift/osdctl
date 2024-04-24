package dynatrace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
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

	hcpLogsCmd.Flags().IntVar(&since, "since", 10, "Number of hours (integer) since which to pull logs")
	hcpLogsCmd.Flags().IntVar(&tail, "tail", 100, "Last 'n' logs to fetch ")
	hcpLogsCmd.Flags().StringVar(&sortOrder, "sort", "desc", "Sort the results by timestamp in either ascending or descending order. Accepted values are 'asc' and 'desc'")

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

	err = dumpPodLogs(pods, logsDir, hcpNS, managementClusterName, DTURL, accessToken, since, tail, sortOrder)
	if err != nil {
		return err
	}

	return nil
}

func dumpPodLogs(pods *corev1.PodList, logsDir string, hcpNS string, managementClusterName string, DTURL string, accessToken string, since int, tail int, sortOrder string) error {
	totalPods := len(pods.Items)
	for k, p := range pods.Items {
		fmt.Println(fmt.Sprintf("[%d/%d] Pod logs for %s", k+1, totalPods, p.Name))
		podLogsQuery, err := getPodQuery(p.Name, hcpNS, since, tail, sortOrder, managementClusterName)
		if err != nil {
			return err
		}
		podLogsQuery.Build()

		requestToken, err := getRequestToken(podLogsQuery.finalQuery, DTURL, accessToken)
		if err != nil {
			return fmt.Errorf("failed to acquire request token %v", err)
		}

		podDirPath, err := addPodDir(logsDir, p.Name)
		if err != nil {
			return err
		}

		podYamlFilePath := filepath.Join(podDirPath, "pod.yaml")
		podYaml, err := yaml.Marshal(p)
		f, err := os.OpenFile(podYamlFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0655)
		if err != nil {
			return err
		}
		f.Write(podYaml)
		f.Close()

		podLogsFilePath := filepath.Join(podDirPath, "pod.log")
		f, err = os.OpenFile(podLogsFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0655)
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
	podLogPath := filepath.Join(podPath, "pod.log")
	_, err = os.Create(podLogPath)

	podYamlPath := filepath.Join(podPath, "pod.yaml")
	_, err = os.Create(podYamlPath)

	return podPath, nil
}

func getPodQuery(pod string, namespace string, since int, tail int, sortOrder string, srcCluster string) (query DTQuery, error error) {
	q := DTQuery{}
	q.Init(since).Cluster(srcCluster)

	if namespace != "" {
		q.Namespaces([]string{namespace})
	}

	if pod != "" {
		q.Pods([]string{pod})
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

func getPodsForNamespace(clientset *kubernetes.Clientset, namespace string) (pl *corev1.PodList, error error) {
	// Getting pod objects for non-running state pod
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace '%s'", namespace)
	}

	return pods, nil
}
