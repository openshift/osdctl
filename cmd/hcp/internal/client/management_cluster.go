package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	veleroNamespace          = "openshift-adp"
	veleroAPIGroup           = "velero.io"
	veleroAPIVersion         = "v1"
	restoreManagedLabel      = "osdctl.openshift.io/managed"
	restoreManagedValue      = "hcp-backup-restore"
	restoreBackupAnnotation  = "osdctl.openshift.io/backup-name"
	restoreClusterAnnotation = "osdctl.openshift.io/cluster-id"
	restoreAttemptAnnotation = "osdctl.openshift.io/attempt"
)

// VeleroRestoreRequest identifies a backup to restore on a management cluster.
type VeleroRestoreRequest struct {
	ClusterID    string
	BackupName   string
	VerifyBackup bool
}

// VeleroRestoreResult reports the managed restore selected or created.
type VeleroRestoreResult struct {
	Name    string
	Created bool
}

// Management cluster client operations are initialized with Kubernetes access.
// The methods only perform Velero actions against that existing connection.
type managementClusterClient struct {
	client    client.Client
	restCfg   *rest.Config
	clientset kubernetes.Interface
}

func (c *managementClusterClient) GetVeleroSchedule(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	schedule := &unstructured.Unstructured{}
	schedule.SetGroupVersionKind(schema.GroupVersionKind{Group: "velero.io", Version: "v1", Kind: "Schedule"})
	if err := c.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, schedule); err != nil {
		return nil, fmt.Errorf("getting Velero schedule %q in namespace %q: %w", name, namespace, err)
	}
	return schedule, nil
}

// ListVolumeSnapshotContents returns all CSI snapshot contents visible to the
// management cluster. The caller is responsible for selecting its HCP's items.
func (c *managementClusterClient) ListVolumeSnapshotContents(ctx context.Context) (*unstructured.UnstructuredList, error) {
	all := &unstructured.UnstructuredList{}
	all.SetGroupVersionKind(schema.GroupVersionKind{Group: "snapshot.storage.k8s.io", Version: "v1", Kind: "VolumeSnapshotContentList"})
	var continuation string
	for {
		contents := &unstructured.UnstructuredList{}
		contents.SetGroupVersionKind(all.GroupVersionKind())
		if err := c.client.List(ctx, contents, client.Limit(500), client.Continue(continuation)); err != nil {
			return nil, fmt.Errorf("listing VolumeSnapshotContents: %w", err)
		}
		all.Items = append(all.Items, contents.Items...)
		continuation = contents.GetContinue()
		if continuation == "" {
			return all, nil
		}
	}
}

func (c *managementClusterClient) CreateVeleroBackup(ctx context.Context, request VeleroBackupRequest) (string, error) {
	pods := &corev1.PodList{}
	if err := c.client.List(ctx, pods, client.InNamespace(request.Namespace), client.MatchingLabels{request.LabelKey: request.LabelValue}); err != nil {
		return "", fmt.Errorf("listing Velero pods in namespace %s: %w", request.Namespace, err)
	}
	podName, err := readyPodName(pods.Items, request.Namespace, request.LabelKey, request.LabelValue)
	if err != nil {
		return "", err
	}
	if c.restCfg == nil || c.clientset == nil {
		return "", fmt.Errorf("creating Velero backup client: Kubernetes exec configuration is unavailable")
	}

	command := []string{"./velero", "backup", "create", "--from-schedule", request.Schedule}
	if len(request.Labels) > 0 {
		command = append(command, "--labels", joinSortedMap(request.Labels))
	}
	if len(request.Annotations) > 0 {
		command = append(command, "--annotations", joinSortedMap(request.Annotations))
	}

	req := c.clientset.CoreV1().RESTClient().Post().Resource("pods").Name(podName).Namespace(request.Namespace).SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{Container: request.Container, Command: command, Stdout: true, Stderr: true}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(c.restCfg, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("creating SPDY executor for pod exec: %w", err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: stdout, Stderr: stderr}); err != nil {
		return "", fmt.Errorf("executing command in pod %q: %w\nstderr: %s", podName, err, stderr.String())
	}
	return stdout.String(), nil
}

func readyPodName(pods []corev1.Pod, namespace, labelKey, labelValue string) (string, error) {
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return pod.Name, nil
			}
		}
	}
	return "", fmt.Errorf("no running Velero pod found in namespace %s with label %s=%s", namespace, labelKey, labelValue)
}

func (c *managementClusterClient) RestoreVeleroBackup(ctx context.Context, request VeleroRestoreRequest) (VeleroRestoreResult, error) {
	resources := veleroResources()
	if err := c.verifyBackupStorageLocation(ctx, resources); err != nil {
		return VeleroRestoreResult{}, err
	}
	if request.VerifyBackup {
		if err := c.verifyBackup(ctx, resources, request.BackupName); err != nil {
			return VeleroRestoreResult{}, err
		}
	}

	restore, created, err := c.getOrCreateRestore(ctx, resources, request.ClusterID, request.BackupName)
	if err != nil {
		return VeleroRestoreResult{}, err
	}
	if err := c.waitForRestore(ctx, resources, restore.GetName(), request.BackupName); err != nil {
		return VeleroRestoreResult{}, err
	}
	return VeleroRestoreResult{Name: restore.GetName(), Created: created}, nil
}

func veleroResources() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: veleroAPIGroup, Version: veleroAPIVersion}
}

func veleroObject(resources schema.GroupVersionKind, kind, name string) *unstructured.Unstructured {
	object := &unstructured.Unstructured{}
	object.SetGroupVersionKind(schema.GroupVersionKind{Group: resources.Group, Version: resources.Version, Kind: kind})
	object.SetNamespace(veleroNamespace)
	object.SetName(name)
	return object
}

func veleroList(resources schema.GroupVersionKind, kind string) *unstructured.UnstructuredList {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{Group: resources.Group, Version: resources.Version, Kind: kind + "List"})
	return list
}

func (c *managementClusterClient) verifyBackupStorageLocation(ctx context.Context, resources schema.GroupVersionKind) error {
	locations := veleroList(resources, "BackupStorageLocation")
	if err := c.client.List(ctx, locations, client.InNamespace(veleroNamespace)); err != nil {
		return fmt.Errorf("listing BackupStorageLocations: %w", err)
	}
	for _, location := range locations.Items {
		phase, _, _ := unstructured.NestedString(location.Object, "status", "phase")
		if phase == "Available" {
			return nil
		}
	}
	return fmt.Errorf("no available BackupStorageLocation in namespace %s", veleroNamespace)
}

func (c *managementClusterClient) verifyBackup(ctx context.Context, resources schema.GroupVersionKind, backupName string) error {
	backup := veleroObject(resources, "Backup", backupName)
	if err := c.client.Get(ctx, types.NamespacedName{Namespace: veleroNamespace, Name: backupName}, backup); err != nil {
		return fmt.Errorf("getting Velero Backup %s: %w", backupName, err)
	}
	phase, _, _ := unstructured.NestedString(backup.Object, "status", "phase")
	if phase != "Completed" {
		return fmt.Errorf("velero Backup %s is %q, expected Completed", backupName, phase)
	}
	return nil
}

func (c *managementClusterClient) getOrCreateRestore(ctx context.Context, resources schema.GroupVersionKind, clusterID, backupName string) (*unstructured.Unstructured, bool, error) {
	restores := veleroList(resources, "Restore")
	if err := c.client.List(ctx, restores, client.InNamespace(veleroNamespace), client.MatchingLabels{restoreManagedLabel: restoreManagedValue}); err != nil {
		return nil, false, fmt.Errorf("listing Velero Restores: %w", err)
	}

	maxAttempt := 0
	for i := range restores.Items {
		restore := &restores.Items[i]
		if restore.GetAnnotations()[restoreBackupAnnotation] != backupName || restore.GetAnnotations()[restoreClusterAnnotation] != clusterID {
			continue
		}
		attempt, _ := strconv.Atoi(restore.GetAnnotations()[restoreAttemptAnnotation])
		maxAttempt = max(maxAttempt, attempt)
		phase, _, _ := unstructured.NestedString(restore.Object, "status", "phase")
		if phase == "Completed" || !terminalRestorePhase(phase) {
			return restore.DeepCopy(), false, nil
		}
	}

	restore := newVeleroRestore(resources, clusterID, backupName, maxAttempt+1)
	if err := c.client.Create(ctx, restore); err != nil {
		return nil, false, fmt.Errorf("creating Velero Restore %s for backup %s: %w", restore.GetName(), backupName, err)
	}
	return restore, true, nil
}

func (c *managementClusterClient) waitForRestore(ctx context.Context, resources schema.GroupVersionKind, restoreName, backupName string) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		restore := veleroObject(resources, "Restore", restoreName)
		if err := c.client.Get(ctx, types.NamespacedName{Namespace: veleroNamespace, Name: restoreName}, restore); err != nil {
			return fmt.Errorf("getting Velero Restore %s for backup %s: %w", restoreName, backupName, err)
		}
		phase, _, _ := unstructured.NestedString(restore.Object, "status", "phase")
		if phase == "Completed" {
			return nil
		}
		if terminalRestorePhase(phase) {
			return fmt.Errorf("velero Restore %s for backup %s reached %s", restoreName, backupName, phase)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for Velero Restore %s for backup %s: %w", restoreName, backupName, ctx.Err())
		case <-ticker.C:
		}
	}
}

func newVeleroRestore(resources schema.GroupVersionKind, clusterID, backupName string, attempt int) *unstructured.Unstructured {
	sum := sha256.Sum256([]byte(clusterID + "\x00" + backupName))
	name := fmt.Sprintf("osdctl-restore-%x-%d", sum[:6], attempt)
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": resources.GroupVersion().String(), "kind": "Restore",
		"metadata": map[string]any{"name": name, "namespace": veleroNamespace, "labels": map[string]any{restoreManagedLabel: restoreManagedValue}, "annotations": map[string]any{restoreBackupAnnotation: backupName, restoreClusterAnnotation: clusterID, restoreAttemptAnnotation: strconv.Itoa(attempt)}},
		"spec":     map[string]any{"backupName": backupName},
	}}
}

func terminalRestorePhase(phase string) bool {
	switch phase {
	case "Failed", "FailedValidation", "PartiallyFailed":
		return true
	default:
		return false
	}
}
