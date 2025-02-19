package network

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RebuildOVNOptions structure definition
type RebuildOVNOptions struct {
	streams genericclioptions.IOStreams
	reason  string
}

// newRebuildOVNOptions creates an instance of RebuildOVNOptions with CLI input/output streams.
func newRebuildOVNOptions(streams genericclioptions.IOStreams) RebuildOVNOptions {
	opts := RebuildOVNOptions{
		streams: streams,
	}
	return opts
}

// newCmdRebuildOVN initializes and returns a Cobra command for rebuilding the OVN stack.
func newCmdRebuildOVN(streams genericclioptions.IOStreams) *cobra.Command {
	ops := newRebuildOVNOptions(streams)
	rebuildOVNCmd := &cobra.Command{
		Use:               "rebuild-ovn $CLUSTER_ID",
		Aliases:           []string{"refresh-ovn"},
		Short:             "Rebuild the OVN stack",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("running ovn rebuild")
			rebuildOVN(cmd, args)
		},
	}
	rebuildOVNCmd.Flags().StringVar(&ops.reason, "reason", "", "The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)")
	rebuildOVNCmd.MarkFlagRequired("reason")
	return rebuildOVNCmd
}

// rebuildOVN initializes the Kubernetes client and executes the OVN reset steps.
func rebuildOVN(cmd *cobra.Command, args []string) {
	clusterID := args[0]
	reason, _ := cmd.Flags().GetString("reason")

	log.Printf("Starting OVN rebuild for cluster: %s with reason: %s", clusterID, reason)

	kubeClient, err := k8s.NewAsBackplaneClusterAdmin(clusterID, client.Options{}, reason)
	if err != nil {
		log.Fatalf("Error getting Kubernetes client: %v", err)
	}

	nodes, err := listOVNKubeNodes(kubeClient)
	if err != nil {
		log.Fatalf("Error listing OVN kube nodes: %v", err)
	}

	log.Printf("Nodes to rebuild: %v", nodes)

	commands := []string{
		"chroot /host /bin/bash -c 'rm -f /var/lib/ovn-ic/etc/ovn*.db && systemctl restart ovs-vswitchd ovsdb-server'",
	}

	for _, node := range nodes {
		log.Printf("Processing node: %s", node)
		if err := executeCommandOnNode(kubeClient, node, commands); err != nil {
			log.Printf("Error executing commands on node %s: %v", node, err)
		}

		// Add a sleep time before deleting the ovnkube-node pod
		time.Sleep(5 * time.Second) // Wait for 5 seconds

		if err := deleteOVNKubeNodePod(kubeClient, node); err != nil {
			log.Printf("Error deleting ovnkube-node pod on node %s: %v", node, err)
		}
		if err := waitForPodRecreation(kubeClient, node); err != nil {
			log.Printf("Error waiting for pod recreation on node %s: %v", node, err)
		}
	}

	// Print success message
	fmt.Println("\033[32mOVN rebuild successful for all nodes\033[0m")
}

// listOVNKubeNodes retrieves the ovnkube-node pods that are running on a specific node.
func listOVNKubeNodes(kubeClient client.Client) ([]string, error) {
	var podList corev1.PodList

	log.Printf("Listing OVN kube nodes")

	// Create label selector
	labelSelector := labels.Set{"app": "ovnkube-node"}.AsSelector()

	log.Printf("Label selector: %s", labelSelector.String())

	err := kubeClient.List(context.TODO(), &podList, &client.ListOptions{
		Namespace:     "openshift-ovn-kubernetes",
		LabelSelector: labelSelector,
	})
	if err != nil {
		log.Printf("Error listing ovnkube-node pods: %v", err)
		return nil, fmt.Errorf("error listing ovnkube-node pods: %w", err)
	}

	log.Printf("Found %d pods", len(podList.Items))

	if len(podList.Items) == 0 {
		log.Printf("No pods found with label selector: %s", labelSelector.String())
	}

	// Create a map to store the nodes and avoid duplicates
	nodes := make(map[string]bool)
	for _, pod := range podList.Items {
		log.Printf("Found pod %s on node %s", pod.Name, pod.Spec.NodeName)
		nodes[pod.Spec.NodeName] = true
	}

	nodeList := make([]string, 0, len(nodes))
	for node := range nodes {
		nodeList = append(nodeList, node)
	}

	log.Printf("Found nodes: %v", nodeList)
	return nodeList, nil
}

// deleteOVNKubeNodePod deletes the ovnkube-node pod on the specified node.
func deleteOVNKubeNodePod(kubeClient client.Client, nodeName string) error {
	var podList corev1.PodList

	log.Printf("Deleting OVN kube node pod on node: %s", nodeName)

	err := kubeClient.List(context.TODO(), &podList, client.InNamespace("openshift-ovn-kubernetes"), client.MatchingLabels{"app": "ovnkube-node"})
	if err != nil {
		return fmt.Errorf("error listing ovnkube-node pods: %w", err)
	}

	for _, pod := range podList.Items {
		if pod.Spec.NodeName == nodeName {
			if err := kubeClient.Delete(context.TODO(), &pod); err != nil {
				return fmt.Errorf("error deleting pod %s: %w", pod.Name, err)
			}
			log.Printf("Deleted ovnkube-node pod %s on node %s", pod.Name, nodeName)
		}
	}
	return nil
}

// executeCommandOnNode creates a privileged pod on the specified node to execute commands.
func executeCommandOnNode(kubeClient client.Client, nodeName string, commands []string) error {
	log.Printf("Creating pod on node %s to execute commands: %v", nodeName, commands)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "ovn-reset-",
			Namespace:    "openshift-ovn-kubernetes", // Change namespace to "openshift-ovn-kubernetes"
		},
		Spec: corev1.PodSpec{
			HostPID:       true,
			NodeName:      nodeName,
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "ovn-reset",
					Image: "registry.redhat.io/ubi9/ubi:latest",
					SecurityContext: &corev1.SecurityContext{
						Privileged: func(b bool) *bool { return &b }(true),
					},
					Command: []string{"/bin/bash"},
					Args:    []string{"-c", commands[0]},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "host-root",
							MountPath: "/host",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "host-root",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/",
						},
					},
				},
			},
		},
	}

	if err := kubeClient.Create(context.TODO(), pod); err != nil {
		return fmt.Errorf("failed to create pod on node %s: %w", nodeName, err)
	}

	log.Printf("Pod created on node %s, waiting for execution...", nodeName)

	err := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 5*time.Minute, false, func(context.Context) (done bool, err error) {
		err = kubeClient.Get(context.Background(), client.ObjectKey{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		}, pod)
		if err != nil {
			log.Printf("failed to get pod '%s/%s': %v", pod.Namespace, pod.Name, err)
			return false, nil
		}
		if pod.Status.Phase == corev1.PodSucceeded {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed waiting for pod '%s/%s' to complete: %w", pod.Namespace, pod.Name, err)
	}

	if err := kubeClient.Delete(context.TODO(), pod); err != nil {
		log.Printf("Error deleting pod on node %s: %v", nodeName, err)
	}

	log.Printf("Pod deleted on node %s", nodeName)
	return nil
}

// waitForPodRecreation waits for the ovnkube-node pod to be recreated and become Running.
func waitForPodRecreation(kubeClient client.Client, nodeName string) error {
	log.Printf("Waiting for ovnkube-node pod recreation on %s...", nodeName)
	for {
		var podList corev1.PodList
		err := kubeClient.List(context.TODO(), &podList, client.InNamespace("openshift-ovn-kubernetes"), client.MatchingLabels{"app": "ovnkube-node"})
		if err != nil {
			return fmt.Errorf("error listing ovnkube-node pods: %w", err)
		}

		for _, pod := range podList.Items {
			if pod.Spec.NodeName == nodeName && pod.Status.Phase == corev1.PodRunning {
				log.Printf("ovnkube-node pod is running on node %s", nodeName)
				return nil
			}
		}
		time.Sleep(5 * time.Second)
	}
}
