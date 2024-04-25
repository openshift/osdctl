package dynatrace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewCmdHCPMustGather() *cobra.Command {
	hcpMgCmd := &cobra.Command{
		Use:               "must-gather <cluster-id>",
		Aliases:           []string{"mg"},
		Short:             "Gather all Pod logs and Application event from HCP",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			err := mustGather(args[0])
			if err != nil {
				cmdutil.CheckErr(err)
			}
		},
	}

	hcpMgCmd.Flags().IntVar(&since, "since", 10, "Number of hours (integer) since which to pull logs and events")
	hcpMgCmd.Flags().IntVar(&tail, "tail", 100, "Last 'n' logs and events to fetch ")
	hcpMgCmd.Flags().StringVar(&sortOrder, "sort", "desc", "Sort the results by timestamp in either ascending or descending order. Accepted values are 'asc' and 'desc'")

	return hcpMgCmd
}

func mustGather(clusterID string) (error error) {
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

	gatherDir, err := setupGatherDir(hcpNS)
	if err != nil {
		return err
	}

	pods, err := getPodsForNamespace(clientset, hcpNS)
	if err != nil {
		return err
	}

	err = dumpPodLogs(pods, gatherDir, hcpNS, managementClusterName, DTURL, accessToken, since, tail, sortOrder)
	if err != nil {
		return err
	}

	deployments, err := getDeploymentsForNamespace(clientset, hcpNS)
	if err != nil {
		return err
	}

	err = dumpEvents(deployments, gatherDir, hcpNS, managementClusterName, DTURL, accessToken, since, tail, sortOrder)
	if err != nil {
		return err
	}

	return nil
}

func dumpEvents(deploys *appsv1.DeploymentList, gatherDir string, hcpNS string, managementClusterName string, DTURL string, accessToken string, since int, tail int, sortOrder string) error {
	totalDeployments := len(deploys.Items)
	for k, d := range deploys.Items {
		fmt.Println(fmt.Sprintf("[%d/%d] Deployment events for %s", k+1, totalDeployments, d.Name))

		eventQuery, err := getEventQuery(d.Name, hcpNS, since, tail, sortOrder, managementClusterName)
		if err != nil {
			return err
		}
		eventQuery.Build()

		eventsRequestToken, err := getRequestToken(eventQuery.finalQuery, DTURL, accessToken)
		if err != nil {
			fmt.Println(fmt.Errorf("failed to acquire request token %v", err))
			continue
		}

		eventsDirPath, err := addEventsDir(gatherDir, d.Name)
		if err != nil {
			return err
		}

		deploymentYamlPath := filepath.Join(eventsDirPath, "deployment.yaml")
		deploymentYaml, err := yaml.Marshal(d)
		f, err := os.OpenFile(deploymentYamlPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0655)
		if err != nil {
			return err
		}
		f.Write(deploymentYaml)
		f.Close()

		eventsFilePath := filepath.Join(eventsDirPath, "events.log")
		f, err = os.OpenFile(eventsFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0655)
		if err != nil {
			return err
		}

		err = getEvents(DTURL, accessToken, eventsRequestToken, f)
		defer f.Close()
		if err != nil {
			return fmt.Errorf("failed to get logs %v", err)
		}

	}
	return nil
}

func dumpPodLogs(pods *corev1.PodList, gatherDir string, hcpNS string, managementClusterName string, DTURL string, accessToken string, since int, tail int, sortOrder string) error {
	totalPods := len(pods.Items)
	for k, p := range pods.Items {
		fmt.Println(fmt.Sprintf("[%d/%d] Pod logs for %s", k+1, totalPods, p.Name))

		podLogsQuery, err := getPodQuery(p.Name, hcpNS, since, tail, sortOrder, managementClusterName)
		if err != nil {
			return err
		}
		podLogsQuery.Build()

		podLogsRequestToken, err := getRequestToken(podLogsQuery.finalQuery, DTURL, accessToken)
		if err != nil {
			return fmt.Errorf("failed to acquire request token %v", err)
		}

		podDirPath, err := addPodDir(gatherDir, p.Name)
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

		err = getLogs(DTURL, accessToken, podLogsRequestToken, f)
		defer f.Close()
		if err != nil {
			return fmt.Errorf("failed to get logs %v", err)
		}
	}

	return nil
}

func setupGatherDir(dirName string) (logsDir string, error error) {
	dirPath := filepath.Join(".", fmt.Sprintf("hcp-must-gather-%s", dirName))
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to setup logs directory %v", err)
	}

	return dirPath, nil
}

func addPodDir(logsDir string, podName string) (path string, error error) {
	podPath := filepath.Join(logsDir, "pods", podName)
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

func addEventsDir(logsDir string, deploymentName string) (path string, error error) {
	deploymentPath := filepath.Join(logsDir, "events", deploymentName)
	err := os.MkdirAll(deploymentPath, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to setup app directory %v", err)
	}
	eventsPath := filepath.Join(deploymentPath, "events.log")
	_, err = os.Create(eventsPath)

	deploymentYamlPath := filepath.Join(deploymentPath, "deployment.yaml")
	_, err = os.Create(deploymentYamlPath)

	return deploymentPath, nil
}

func getPodQuery(pod string, namespace string, since int, tail int, sortOrder string, srcCluster string) (query DTQuery, error error) {
	q := DTQuery{}
	q.InitLogs(since).Cluster(srcCluster)

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

func getEventQuery(deploy string, namespace string, since int, tail int, sortOrder string, srcCluster string) (query DTQuery, error error) {
	q := DTQuery{}
	q.InitEvents(since).Cluster(srcCluster)

	if namespace != "" {
		q.Namespaces([]string{namespace})
	}

	if deploy != "" {
		q.Deployments([]string{deploy})
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

func getDeploymentsForNamespace(clientset *kubernetes.Clientset, namespace string) (pl *appsv1.DeploymentList, error error) {
	// Getting pod objects for non-running state pod
	deploys, err := clientset.AppsV1().Deployments(namespace).List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace '%s'", namespace)
	}

	return deploys, nil
}
