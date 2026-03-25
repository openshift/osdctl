package infra

import (
	"context"
	"fmt"
	"log"
	"time"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	pollTimeout             = 20 * time.Minute
	pollInterval            = 20 * time.Second
	InfraNodeLabel          = "node-role.kubernetes.io/infra"
	TemporaryInfraNodeLabel = "osdctl.openshift.io/infra-resize-temporary-machinepool"
)

// GetInfraMachinePool finds the infra MachinePool in Hive for the given cluster ID.
func GetInfraMachinePool(ctx context.Context, hiveClient client.Client, clusterID string) (*hivev1.MachinePool, error) {
	ns := &corev1.NamespaceList{}
	selector, err := labels.Parse(fmt.Sprintf("api.openshift.com/id=%s", clusterID))
	if err != nil {
		return nil, err
	}

	if err := hiveClient.List(ctx, ns, &client.ListOptions{LabelSelector: selector, Limit: 1}); err != nil {
		return nil, err
	}
	if len(ns.Items) != 1 {
		return nil, fmt.Errorf("expected 1 namespace, found %d namespaces with tag: api.openshift.com/id=%s", len(ns.Items), clusterID)
	}

	log.Printf("found namespace: %s", ns.Items[0].Name)

	mpList := &hivev1.MachinePoolList{}
	if err := hiveClient.List(ctx, mpList, &client.ListOptions{Namespace: ns.Items[0].Name}); err != nil {
		return nil, err
	}

	for _, mp := range mpList.Items {
		if mp.Spec.Name == "infra" {
			log.Printf("found machinepool %s", mp.Name)
			return &mp, nil
		}
	}

	return nil, fmt.Errorf("did not find the infra machinepool in namespace: %s", ns.Items[0].Name)
}

// CloneMachinePool deep copies a MachinePool, resets metadata fields so it can
// be created as a new resource, and applies the modifier function.
func CloneMachinePool(mp *hivev1.MachinePool, modifyFn func(*hivev1.MachinePool) error) (*hivev1.MachinePool, error) {
	newMp := &hivev1.MachinePool{}
	mp.DeepCopyInto(newMp)

	newMp.CreationTimestamp = metav1.Time{}
	newMp.Finalizers = []string{}
	newMp.ResourceVersion = ""
	newMp.Generation = 0
	newMp.UID = ""
	newMp.Status = hivev1.MachinePoolStatus{}

	if modifyFn != nil {
		if err := modifyFn(newMp); err != nil {
			return nil, err
		}
	}

	return newMp, nil
}

// DanceClients holds the k8s clients needed for the machinepool dance.
type DanceClients struct {
	ClusterClient client.Client
	HiveClient    client.Client
	HiveAdmin     client.Client
}

// RunMachinePoolDance performs the machinepool dance to replace infra nodes.
// It takes the original MachinePool and an already-modified new MachinePool.
// The dance creates a temporary pool, waits for nodes, deletes the original,
// creates a permanent replacement, then removes the temporary pool.
//
// The onTimeout callback is called when nodes fail to drain within the timeout.
// It receives the list of stuck nodes and should terminate the backing instances.
// If onTimeout is nil, the dance will return an error on timeout.
func RunMachinePoolDance(ctx context.Context, clients DanceClients, originalMp, newMp *hivev1.MachinePool, onTimeout func(ctx context.Context, nodes *corev1.NodeList) error) error {
	tempMp := newMp.DeepCopy()
	tempMp.Name = fmt.Sprintf("%s2", tempMp.Name)
	tempMp.Spec.Name = fmt.Sprintf("%s2", tempMp.Spec.Name)
	tempMp.Spec.Labels[TemporaryInfraNodeLabel] = ""

	// Create the temporary machinepool
	log.Printf("creating temporary machinepool %s", tempMp.Name)
	if err := clients.HiveAdmin.Create(ctx, tempMp); err != nil {
		return err
	}

	// Wait for 2x infra nodes to be Ready
	selector, err := labels.Parse(InfraNodeLabel)
	if err != nil {
		return err
	}

	pollCtx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()
	if err := wait.PollUntilContextTimeout(pollCtx, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		nodes := &corev1.NodeList{}
		if err := clients.ClusterClient.List(ctx, nodes, &client.ListOptions{LabelSelector: selector}); err != nil {
			log.Printf("error retrieving nodes list, continuing to wait: %s", err)
			return false, nil
		}

		readyNodes := countReadyNodes(nodes)
		expected := int(*originalMp.Spec.Replicas) * 2
		log.Printf("waiting for %d infra nodes to be reporting Ready, found %d", expected, readyNodes)

		return readyNodes >= expected, nil
	}); err != nil {
		return err
	}

	// Build selectors for original vs temp nodes
	requireInfra, err := labels.NewRequirement(InfraNodeLabel, selection.Exists, nil)
	if err != nil {
		return err
	}
	requireNotTempNode, err := labels.NewRequirement(TemporaryInfraNodeLabel, selection.DoesNotExist, nil)
	if err != nil {
		return err
	}
	requireTempNode, err := labels.NewRequirement(TemporaryInfraNodeLabel, selection.Exists, nil)
	if err != nil {
		return err
	}

	originalNodeSelector := selector.Add(*requireInfra, *requireNotTempNode)
	tempNodeSelector := selector.Add(*requireInfra, *requireTempNode)

	originalNodes := &corev1.NodeList{}
	if err := clients.ClusterClient.List(ctx, originalNodes, &client.ListOptions{LabelSelector: originalNodeSelector}); err != nil {
		return err
	}

	// Delete original machinepool
	log.Printf("deleting original machinepool %s", originalMp.Name)
	if err := clients.HiveAdmin.Delete(ctx, originalMp); err != nil {
		return err
	}

	// Wait for original machinepool to delete
	if err := waitForMachinePoolDeletion(ctx, clients.HiveClient, originalMp); err != nil {
		return err
	}

	// Wait for original nodes to delete
	if err := waitForNodesDeletion(ctx, clients.ClusterClient, originalNodeSelector, onTimeout, originalNodes); err != nil {
		return err
	}

	// Create new permanent machinepool
	log.Printf("creating new permanent machinepool %s", newMp.Name)
	if err := clients.HiveAdmin.Create(ctx, newMp); err != nil {
		return err
	}

	// Wait for new permanent machines to become nodes
	pollCtx2, cancel2 := context.WithTimeout(ctx, pollTimeout)
	defer cancel2()
	if err := wait.PollUntilContextTimeout(pollCtx2, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		nodes := &corev1.NodeList{}
		infraSelector, err := labels.Parse("node-role.kubernetes.io/infra=")
		if err != nil {
			return false, err
		}
		if err := clients.ClusterClient.List(ctx, nodes, &client.ListOptions{LabelSelector: infraSelector}); err != nil {
			log.Printf("error retrieving nodes list, continuing to wait: %s", err)
			return false, nil
		}

		readyNodes := countReadyNodes(nodes)
		expected := int(*originalMp.Spec.Replicas) * 2
		log.Printf("waiting for %d infra nodes to be reporting Ready, found %d", expected, readyNodes)

		return readyNodes >= expected, nil
	}); err != nil {
		return err
	}

	tempNodes := &corev1.NodeList{}
	if err := clients.ClusterClient.List(ctx, tempNodes, &client.ListOptions{LabelSelector: tempNodeSelector}); err != nil {
		return err
	}

	// Delete temp machinepool
	log.Printf("deleting temporary machinepool %s", tempMp.Name)
	if err := clients.HiveAdmin.Delete(ctx, tempMp); err != nil {
		return err
	}

	// Wait for temporary machinepool to delete
	if err := waitForMachinePoolDeletion(ctx, clients.HiveClient, tempMp); err != nil {
		return err
	}

	// Wait for infra node count to return to normal
	log.Printf("waiting for infra node count to return to: %d", int(*originalMp.Spec.Replicas))
	pollCtx3, cancel3 := context.WithTimeout(ctx, pollTimeout)
	defer cancel3()
	if err := wait.PollUntilContextTimeout(pollCtx3, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		nodes := &corev1.NodeList{}
		infraSelector, err := labels.Parse("node-role.kubernetes.io/infra=")
		if err != nil {
			return false, err
		}
		if err := clients.ClusterClient.List(ctx, nodes, &client.ListOptions{LabelSelector: infraSelector}); err != nil {
			log.Printf("error retrieving nodes list, continuing to wait: %s", err)
			return false, nil
		}

		switch len(nodes.Items) {
		case int(*originalMp.Spec.Replicas):
			log.Printf("found %d infra nodes, replacement complete", len(nodes.Items))
			return true, nil
		default:
			log.Printf("found %d infra nodes, continuing to wait", len(nodes.Items))
			return false, nil
		}
	}); err != nil {
		if wait.Interrupted(err) && onTimeout != nil {
			log.Printf("Warning: timed out waiting for nodes to drain: %v. Terminating backing cloud instances.", err.Error())
			if err := onTimeout(ctx, tempNodes); err != nil {
				return err
			}
			if err := waitForNodesGone(ctx, clients.ClusterClient, tempNodeSelector); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

func waitForMachinePoolDeletion(ctx context.Context, hiveClient client.Client, mp *hivev1.MachinePool) error {
	pollCtx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()
	return wait.PollUntilContextTimeout(pollCtx, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		existing := &hivev1.MachinePool{}
		err := hiveClient.Get(ctx, client.ObjectKey{Namespace: mp.Namespace, Name: mp.Name}, existing)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			log.Printf("error checking machinepool %s/%s, continuing to wait: %s", mp.Namespace, mp.Name, err)
			return false, nil
		}
		log.Printf("machinepool %s/%s still exists, continuing to wait", mp.Namespace, mp.Name)
		return false, nil
	})
}

func waitForNodesDeletion(ctx context.Context, clusterClient client.Client, selector labels.Selector, onTimeout func(ctx context.Context, nodes *corev1.NodeList) error, originalNodes *corev1.NodeList) error {
	pollCtx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()
	if err := wait.PollUntilContextTimeout(pollCtx, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		return nodesMatchExpectedCount(ctx, clusterClient, selector, 0)
	}); err != nil {
		if wait.Interrupted(err) && onTimeout != nil {
			log.Printf("Warning: timed out waiting for nodes to drain: %v. Terminating backing cloud instances.", err.Error())
			if err := onTimeout(ctx, originalNodes); err != nil {
				return err
			}
			return waitForNodesGone(ctx, clusterClient, selector)
		}
		return err
	}
	return nil
}

func waitForNodesGone(ctx context.Context, clusterClient client.Client, selector labels.Selector) error {
	pollCtx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()
	return wait.PollUntilContextTimeout(pollCtx, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		log.Printf("waiting for nodes to terminate")
		match, err := nodesMatchExpectedCount(ctx, clusterClient, selector, 0)
		if err != nil {
			log.Printf("error matching expected count, continuing to wait: %s", err)
			return false, nil
		}
		return match, nil
	})
}

func nodesMatchExpectedCount(ctx context.Context, clusterClient client.Client, labelSelector labels.Selector, count int) (bool, error) {
	nodeList := &corev1.NodeList{}
	if err := clusterClient.List(ctx, nodeList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return false, err
	}
	return len(nodeList.Items) == count, nil
}

func countReadyNodes(nodes *corev1.NodeList) int {
	ready := 0
	for _, node := range nodes.Items {
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready++
				log.Printf("found node %s reporting Ready", node.Name)
			}
		}
	}
	return ready
}
