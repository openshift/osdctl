package dynatrace

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/openshift/osdctl/cmd/common"
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
		Use:               "gather-logs <cluster-id>",
		Aliases:           []string{"gl"},
		Short:             "Gather all Pod logs and Application event from HCP",
		Long:              "This command gathers pods logs and evnets of a given HCP from Dynatrace. It will fetch the logs from the HCP namespace, the hypershift namespace and cert-manager related namespaces. Logs will be dumped to a directory with prefix hcp-must-gather.",
		Example:           "osdctl cluster dynatrace gather-logs hcp-cluster-id-123",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			err := gatherLogs(args[0])
			if err != nil {
				cmdutil.CheckErr(err)
			}
		},
	}

	hcpMgCmd.Flags().IntVar(&since, "since", 10, "Number of hours (integer) since which to pull logs and events")
	hcpMgCmd.Flags().IntVar(&tail, "tail", 0, "Last 'n' logs and events to fetch. By default it will pull everything")
	hcpMgCmd.Flags().StringVar(&sortOrder, "sort", "desc", "Sort the results by timestamp in either ascending or descending order. Accepted values are 'asc' and 'desc'")

	return hcpMgCmd
}

func gatherLogs(clusterID string) (error error) {
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

	_, _, clientset, err := common.GetKubeConfigAndClient(managementClusterInternalID, "", "")
	if err != nil {
		return fmt.Errorf("failed to retrieve Kubernetes configuration and client for cluster with ID %s: %w", managementClusterInternalID, err)
	}

	klusterletNS, shortNS, hcpNS, err := GetHCPNamespacesFromInternalID(clientset, clusterInternalID)
	if err != nil {
		return err
	}

	fmt.Println(fmt.Sprintf("Using HCP Namespace %v", hcpNS))

	gatherNamespaces := []string{hcpNS, klusterletNS, shortNS, "hypershift", "cert-manager", "redhat-cert-manager-operator"}
	gatherDir, err := setupGatherDir(hcpNS)
	if err != nil {
		return err
	}

	for _, gatherNS := range gatherNamespaces {
		fmt.Println(fmt.Sprintf("Gathering for %s", gatherNS))

		pods, err := getPodsForNamespace(clientset, gatherNS)
		if err != nil {
			return err
		}

		nsDir, err := addDir([]string{gatherDir, gatherNS}, []string{})
		if err != nil {
			return err
		}

		err = dumpPodLogs(pods, nsDir, gatherNS, managementClusterName, DTURL, accessToken, since, tail, sortOrder)
		if err != nil {
			return err
		}

		deployments, err := getDeploymentsForNamespace(clientset, gatherNS)
		if err != nil {
			return err
		}

		err = dumpEvents(deployments, nsDir, gatherNS, managementClusterName, DTURL, accessToken, since, tail, sortOrder)
		if err != nil {
			return err
		}

	}

	return nil
}

func dumpEvents(deploys *appsv1.DeploymentList, parentDir string, targetNS string, managementClusterName string, DTURL string, accessToken string, since int, tail int, sortOrder string) error {
	totalDeployments := len(deploys.Items)
	for k, d := range deploys.Items {
		fmt.Println(fmt.Sprintf("[%d/%d] Deployment events for %s", k+1, totalDeployments, d.Name))

		eventQuery, err := getEventQuery(d.Name, targetNS, since, tail, sortOrder, managementClusterName)
		if err != nil {
			return err
		}
		eventQuery.Build()

		deploymentYamlFileName := "deployment.yaml"
		eventsFileName := "events.log"
		eventsDirPath, err := addDir([]string{parentDir, "events", d.Name}, []string{deploymentYamlFileName, eventsFileName})
		if err != nil {
			return err
		}

		deploymentYamlPath := filepath.Join(eventsDirPath, deploymentYamlFileName)
		deploymentYaml, err := yaml.Marshal(d)
		f, err := os.OpenFile(deploymentYamlPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0655)
		if err != nil {
			return err
		}
		f.Write(deploymentYaml)
		f.Close()

		eventsFilePath := filepath.Join(eventsDirPath, eventsFileName)
		f, err = os.OpenFile(eventsFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0655)
		if err != nil {
			return err
		}

		eventsRequestToken, err := getDTQueryExecution(DTURL, accessToken, eventQuery.finalQuery)
		err = getEvents(DTURL, accessToken, eventsRequestToken, f)
		f.Close()
		if err != nil {
			log.Printf("failed to get logs, continuing: %v. Query: %v", err, eventQuery.finalQuery)
			continue
		}

	}
	return nil
}

func dumpPodLogs(pods *corev1.PodList, parentDir string, targetNS string, managementClusterName string, DTURL string, accessToken string, since int, tail int, sortOrder string) error {
	totalPods := len(pods.Items)
	for k, p := range pods.Items {
		fmt.Println(fmt.Sprintf("[%d/%d] Pod logs for %s", k+1, totalPods, p.Name))

		podLogsQuery, err := getPodQuery(p.Name, targetNS, since, tail, sortOrder, managementClusterName)
		if err != nil {
			return err
		}
		podLogsQuery.Build()

		podYamlFileName := "pod.yaml"
		podLogFileName := "pod.log"
		podDirPath, err := addDir([]string{parentDir, "pods", p.Name}, []string{podLogFileName, podYamlFileName})
		if err != nil {
			return err
		}

		podYamlFilePath := filepath.Join(podDirPath, podYamlFileName)
		podYaml, err := yaml.Marshal(p)
		f, err := os.OpenFile(podYamlFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0655)
		if err != nil {
			return err
		}
		f.Write(podYaml)
		f.Close()

		podLogsFilePath := filepath.Join(podDirPath, podLogFileName)
		f, err = os.OpenFile(podLogsFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0655)
		if err != nil {
			return err
		}

		podLogsRequestToken, err := getDTQueryExecution(DTURL, accessToken, podLogsQuery.finalQuery)
		err = getLogs(DTURL, accessToken, podLogsRequestToken, f)
		f.Close()
		if err != nil {
			log.Printf("failed to get logs, continuing: %v. Query: %v", err, podLogsQuery.finalQuery)
			continue
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

func addDir(dirs []string, filePaths []string) (path string, error error) {
	dirPath := filepath.Join(dirs...)
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to setup directory %v", err)
	}
	for _, fp := range filePaths {
		createdFile := filepath.Join(dirPath, fp)
		_, err = os.Create(createdFile)
		if err != nil {
			return "", fmt.Errorf("file to create file %v in %v", fp, err)
		}
	}

	return dirPath, nil
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
