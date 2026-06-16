package cluster

import (
	"context"
	"fmt"
	"log"
	"time"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/backplane-cli/pkg/ocm"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterClients holds the various Kubernetes clients needed for cluster operations.
type ClusterClients struct {
	Client          client.Client
	ClientAdmin     client.Client
	HiveClient      client.Client
	HiveAdminClient client.Client
}

// SetupClusterClients initializes standard Kubernetes clients for a cluster.
func SetupClusterClients(clusterID, reason, operation string) (*ClusterClients, error) {
	scheme := runtime.NewScheme()

	// Register Machine API v1 for Machine resources
	if err := machinev1.Install(scheme); err != nil {
		return nil, err
	}

	// Register Machine API v1beta1 for MachineSet and MachineHealthCheck resources
	if err := machinev1beta1.Install(scheme); err != nil {
		return nil, err
	}

	// Register core v1 API for Pods, Nodes, ConfigMaps, etc.
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	// Register config v1 API for ClusterOperator resources
	if err := configv1.Install(scheme); err != nil {
		return nil, err
	}

	// Create standard Kubernetes client (read-only)
	c, err := k8s.New(clusterID, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	// Create elevated cluster-admin client for mutations
	cAdmin, err := k8s.NewAsBackplaneClusterAdmin(clusterID, client.Options{Scheme: scheme}, []string{
		reason,
		fmt.Sprintf("%s for cluster %s", operation, clusterID),
	}...)
	if err != nil {
		return nil, err
	}

	return &ClusterClients{
		Client:      c,
		ClientAdmin: cAdmin,
	}, nil
}

// SetupHiveClients initializes Hive clients for MachinePool operations.
func SetupHiveClients(clusterID, reason, operation string) (hiveClient, hiveAdminClient client.Client, err error) {
	// Create scheme for Hive API resources
	hiveScheme := runtime.NewScheme()
	if err := hivev1.AddToScheme(hiveScheme); err != nil {
		return nil, nil, err
	}
	if err := corev1.AddToScheme(hiveScheme); err != nil {
		return nil, nil, err
	}

	// Get the Hive management cluster
	hive, err := utils.GetHiveCluster(clusterID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get hive cluster: %v", err)
	}

	// Create read-only Hive client
	hc, err := k8s.New(hive.ID(), client.Options{Scheme: hiveScheme})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create hive client: %v", err)
	}

	// Create elevated Hive client for MachinePool mutations
	hac, err := k8s.NewAsBackplaneClusterAdmin(hive.ID(), client.Options{Scheme: hiveScheme}, []string{
		reason,
		fmt.Sprintf("%s for cluster %s", operation, clusterID),
	}...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create hive admin client: %v", err)
	}

	return hc, hac, nil
}

// CheckClusterOperators verifies all cluster operators are healthy.
func CheckClusterOperators(ctx context.Context, c client.Client) error {
	coList := &configv1.ClusterOperatorList{}
	if err := c.List(ctx, coList); err != nil {
		return fmt.Errorf("failed to list clusteroperators: %w", err)
	}

	var unhealthyOps []string
	for _, op := range coList.Items {
		available, degraded := false, false
		for _, cond := range op.Status.Conditions {
			switch cond.Type {
			case configv1.OperatorAvailable:
				available = cond.Status == configv1.ConditionTrue
			case configv1.OperatorDegraded:
				degraded = cond.Status == configv1.ConditionTrue
			}
		}
		if !available || degraded {
			unhealthyOps = append(unhealthyOps, op.Name)
		}
	}

	if len(unhealthyOps) > 0 {
		return fmt.Errorf("unhealthy cluster operators: %v", unhealthyOps)
	}

	fmt.Printf("  ClusterOperators: All healthy\n")
	return nil
}

// CheckCPMSState verifies the ControlPlaneMachineSet is Active and ready.
func CheckCPMSState(ctx context.Context, c client.Client, namespace, name string) error {
	cpms := &machinev1.ControlPlaneMachineSet{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, cpms); err != nil {
		return fmt.Errorf("failed to get CPMS: %v", err)
	}

	if cpms.Spec.State != machinev1.ControlPlaneMachineSetStateActive {
		return fmt.Errorf("CPMS is not Active (state: %s). Cannot proceed with control plane changes", cpms.Spec.State)
	}

	if cpms.Status.ReadyReplicas != 3 {
		return fmt.Errorf("CPMS does not have 3 ready replicas (ready: %d)", cpms.Status.ReadyReplicas)
	}

	fmt.Printf("  CPMS: Active, %d/3 ready\n", cpms.Status.ReadyReplicas)
	return nil
}

// MonitorCPMSRollout polls the CPMS until all replicas are updated.
func MonitorCPMSRollout(ctx context.Context, c client.Client, namespace, name string, timeout time.Duration) error {
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextTimeout(pollCtx, 30*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		cpms := &machinev1.ControlPlaneMachineSet{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, cpms); err != nil {
			log.Printf("Warning: Error checking CPMS status (will retry): %v", err)
			return false, nil
		}

		updated := cpms.Status.UpdatedReplicas
		ready := cpms.Status.ReadyReplicas

		log.Printf("[%s] CPMS: %d/3 updated, %d/3 ready", time.Now().Format("15:04:05"), updated, ready)

		if updated == 3 && ready >= 3 {
			return true, nil
		}
		return false, nil
	})
}

// CountReadyNodes counts the number of Ready nodes in a NodeList.
func CountReadyNodes(nodes *corev1.NodeList) int {
	ready := 0
	for _, node := range nodes.Items {
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready++
				break
			}
		}
	}
	return ready
}

// WaitForClusterOperatorsHealthy waits for all cluster operators to become healthy.
func WaitForClusterOperatorsHealthy(ctx context.Context, c client.Client, timeout time.Duration) error {
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextTimeout(pollCtx, 30*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		if err := CheckClusterOperators(ctx, c); err != nil {
			log.Printf("Cluster operators not yet healthy: %v", err)
			return false, nil
		}
		return true, nil
	})
}

// ValidateAWSClassicCluster validates the cluster is an AWS Classic (non-HCP) cluster.
func ValidateAWSClassicCluster(cluster *cmv1.Cluster) error {
	if cluster.CloudProvider().ID() != "aws" {
		return fmt.Errorf("this command only supports AWS clusters (cluster is %s)", cluster.CloudProvider().ID())
	}

	if cluster.Hypershift().Enabled() {
		return fmt.Errorf("this command does not support HCP clusters")
	}

	return nil
}

// GetHiveNamespace returns the Hive namespace for a given cluster ID.
// This is reusable across multiple cluster commands that interact with Hive.
func GetHiveNamespace(clusterID string) (string, error) {
	env, err := ocm.DefaultOCMInterface.GetOCMEnvironment()
	if err != nil {
		return "", fmt.Errorf("failed to get OCM environment: %w", err)
	}
	return fmt.Sprintf("uhc-%s-%s", env.Name(), clusterID), nil
}
