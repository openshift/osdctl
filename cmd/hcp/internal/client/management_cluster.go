package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/openshift/osdctl/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/retry"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	storageOperatorNamespace = "openshift-cluster-storage-operator"
	clusterVersionGroup      = "config.openshift.io"
	hypershiftGroup          = "hypershift.openshift.io"
	remapPollInterval        = 2 * time.Second
)

// VolumeSnapshotRemapRequest describes the target-cluster VSC replacement
// performed after snapshots have been copied into the target AWS account.
type VolumeSnapshotRemapRequest struct {
	HCPNamespace string
	DomainPrefix string
	Mappings     map[string]string // VSC name to destination EBS snapshot ID.
}

const (
	defaultVeleroNamespace   = "openshift-adp"
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

type ManagementClusterClientConfig struct {
	VeleroNamespace string
	Logger          *logrus.Logger
}

func (c *ManagementClusterClientConfig) Options(opts ...ManagementClusterClientOption) {
	for _, opt := range opts {
		opt.ConfigureManagementClusterClient(c)
	}
}

func (c *ManagementClusterClientConfig) Defaults() {
	if c.VeleroNamespace == "" {
		c.VeleroNamespace = defaultVeleroNamespace
	}
	if c.Logger == nil {
		c.Logger = utils.NewDiscardLogger()
	}
}

type ManagementClusterClientOption interface {
	ConfigureManagementClusterClient(*ManagementClusterClientConfig)
}

// Management cluster client operations are initialized with Kubernetes access.
// The methods only perform Velero actions against that existing connection.
type managementClusterClient struct {
	client    client.Client
	restCfg   *rest.Config
	clientset kubernetes.Interface
	cfg       ManagementClusterClientConfig
}

// NewManagementClusterClient creates a management-cluster client with optional configuration.
func NewManagementClusterClient(kubeClient client.Client, restCfg *rest.Config, clientset kubernetes.Interface, opts ...ManagementClusterClientOption) *managementClusterClient {
	var cfg ManagementClusterClientConfig
	cfg.Options(opts...)
	cfg.Defaults()

	return &managementClusterClient{
		client:    kubeClient,
		restCfg:   restCfg,
		clientset: clientset,
		cfg:       cfg,
	}
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

// RemapVolumeSnapshotContents replaces the supplied VSCs while HyperShift and
// snapshot reconciliation are stopped. It only unpauses the control plane once
// every replacement is ready to use.
func (c *managementClusterClient) RemapVolumeSnapshotContents(ctx context.Context, request VolumeSnapshotRemapRequest) (err error) {
	if request.HCPNamespace == "" || request.DomainPrefix == "" || len(request.Mappings) == 0 {
		return fmt.Errorf("HCP namespace, domain prefix, and VSC mappings are required")
	}
	started := time.Now()
	c.logRemap("Starting VolumeSnapshotContent remapping", logrus.Fields{"hcp_namespace": request.HCPNamespace, "domain_prefix": request.DomainPrefix, "vsc_count": len(request.Mappings)})
	contents, err := c.ListVolumeSnapshotContents(ctx)
	if err != nil {
		return err
	}
	c.logRemap("Listed VolumeSnapshotContents", logrus.Fields{"vsc_count": len(contents.Items)})
	originals, err := remapOriginals(contents, request.HCPNamespace, request.Mappings)
	if err != nil {
		return err
	}
	c.logRemap("Selected VolumeSnapshotContents for remapping", logrus.Fields{"vsc_count": len(originals)})
	if err := c.validateRemapTargets(ctx, request.Mappings); err != nil {
		return err
	}
	c.logRemap("Validated destination snapshot mappings", logrus.Fields{"vsc_count": len(request.Mappings)})
	state, pauseErr := c.pauseSnapshotReconciliation(ctx, request.HCPNamespace, request.DomainPrefix)
	if state == nil {
		return pauseErr
	}
	completed := false
	replacementsStarted := false
	defer func() {
		if !completed {
			c.logRemap("Remapping failed; restoring HostedCluster and snapshot reconciliation", logrus.Fields{"error": err})
			recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			if replacementsStarted {
				stopErr := c.stopSnapshotControllers(recoveryCtx, state)
				if stopErr != nil {
					c.logRemap("Recovery could not stop snapshot controllers; skipping VSC rollback", logrus.Fields{"error": stopErr})
					err = errors.Join(err, fmt.Errorf("stopping snapshot controllers for rollback: %w", stopErr))
				} else if rollbackErr := c.restoreVSCs(recoveryCtx, originals); rollbackErr != nil {
					err = errors.Join(err, fmt.Errorf("restoring original VSCs: %w", rollbackErr))
				} else {
					c.logRemap("Restored original VolumeSnapshotContents", logrus.Fields{"vsc_count": len(originals)})
				}
			}
			if recoveryErr := c.recoverSnapshotReconciliation(recoveryCtx, state); recoveryErr != nil {
				err = errors.Join(err, recoveryErr)
			}
		}
	}()
	if pauseErr != nil {
		return pauseErr
	}
	c.logRemap("Paused snapshot reconciliation", logrus.Fields{"hosted_cluster": state.hostedCluster.GetName()})
	for name, destinationID := range request.Mappings {
		replacementsStarted = true
		c.logRemap("Replacing VolumeSnapshotContent", logrus.Fields{"volume_snapshot_content": name, "source_snapshot": snapshotHandle(originals[name]), "destination_snapshot": destinationID})
		if err := c.replaceVSC(ctx, originals[name], destinationID); err != nil {
			return fmt.Errorf("replacing VolumeSnapshotContent %s: %w", name, err)
		}
		c.logRemap("Created replacement VolumeSnapshotContent", logrus.Fields{"volume_snapshot_content": name, "destination_snapshot": destinationID})
	}
	if err := c.verifyRemappedVSCHandles(ctx, request.Mappings); err != nil {
		return err
	}
	c.logRemap("Verified replacement VolumeSnapshotContent handles", logrus.Fields{"vsc_count": len(request.Mappings)})
	if err := c.resumeSnapshotControllers(ctx, state); err != nil {
		return err
	}
	c.logRemap("Restarted snapshot controllers", nil)
	if err := c.waitForRemappedVSCs(ctx, request.Mappings); err != nil {
		return err
	}
	c.logRemap("Replacement VolumeSnapshotContents are ready to use", logrus.Fields{"vsc_count": len(request.Mappings)})
	if state.hostedClusterPaused {
		if err := c.setPaused(ctx, state.hostedCluster, nil); err != nil {
			return fmt.Errorf("unpausing HostedCluster: %w", err)
		}
	}
	completed = true
	c.logRemap("Completed VolumeSnapshotContent remapping", logrus.Fields{"vsc_count": len(request.Mappings), "duration": time.Since(started).Round(time.Second)})
	return nil
}

type remapState struct {
	hcp, hostedCluster          *unstructured.Unstructured
	deployments                 map[string]int32
	snapshotControllerOverrides []snapshotControllerOverrideChange
	hostedClusterPaused         bool
}

type snapshotControllerOverrideChange struct {
	name              string
	created           bool
	hadUnmanaged      bool
	originalUnmanaged any
}

func remapOriginals(contents *unstructured.UnstructuredList, namespace string, mappings map[string]string) (map[string]*unstructured.Unstructured, error) {
	originals := make(map[string]*unstructured.Unstructured, len(mappings))
	for i := range contents.Items {
		content := &contents.Items[i]
		if _, wanted := mappings[content.GetName()]; !wanted {
			continue
		}
		vscNamespace, _, _ := unstructured.NestedString(content.Object, "spec", "volumeSnapshotRef", "namespace")
		driver, _, _ := unstructured.NestedString(content.Object, "spec", "driver")
		if vscNamespace != namespace || driver != "ebs.csi.aws.com" {
			return nil, fmt.Errorf("VolumeSnapshotContent %s is not an EBS VSC for HCP namespace %s", content.GetName(), namespace)
		}
		originals[content.GetName()] = content.DeepCopy()
	}
	if len(originals) != len(mappings) {
		return nil, fmt.Errorf("not all mapped VolumeSnapshotContents are present on the replacement management cluster")
	}
	return originals, nil
}

func (c *managementClusterClient) validateRemapTargets(ctx context.Context, mappings map[string]string) error {
	for name, handle := range mappings {
		if handle == "" {
			return fmt.Errorf("VolumeSnapshotContent %s has no destination snapshot ID", name)
		}
	}
	return nil
}

func (c *managementClusterClient) pauseSnapshotReconciliation(ctx context.Context, namespace, domainPrefix string) (*remapState, error) {
	hcp, err := c.getHostedControlPlane(ctx, namespace, domainPrefix)
	if err != nil {
		return nil, fmt.Errorf("getting HostedControlPlane: %w", err)
	}
	hc, err := c.findHostedCluster(ctx, domainPrefix)
	if err != nil {
		return nil, fmt.Errorf("getting HostedCluster: %w", err)
	}
	state := &remapState{hcp: hcp.DeepCopy(), hostedCluster: hc.DeepCopy(), deployments: map[string]int32{}}
	c.logRemap("Pausing HostedCluster", logrus.Fields{"hosted_cluster": hc.GetName(), "namespace": hc.GetNamespace()})
	if err := c.setPaused(ctx, hc, ptr("true")); err != nil {
		return state, err
	}
	state.hostedClusterPaused = true
	if err := c.waitForHostedControlPlanePaused(ctx, namespace, domainPrefix); err != nil {
		return state, err
	}
	etcd, err := c.getStatefulSet(ctx, namespace, "etcd")
	if err != nil {
		return state, fmt.Errorf("getting StatefulSet %s/etcd: %w", namespace, err)
	}
	replicas, _, _ := unstructured.NestedInt64(etcd.Object, "spec", "replicas")
	replicaCount, err := int32ReplicaCount(replicas)
	if err != nil {
		return state, fmt.Errorf("getting StatefulSet %s/etcd replicas: %w", namespace, err)
	}
	state.deployments[namespace+"/etcd"] = replicaCount
	c.logWorkloadReplicas("Stopping etcd", etcd, 0)
	if err := c.scaleDeployment(ctx, etcd, 0); err != nil {
		return state, err
	}
	if err := c.waitForWorkloadReplicas(ctx, "StatefulSet", namespace, "etcd", 0); err != nil {
		return state, err
	}
	version, err := c.getClusterVersion(ctx)
	if err != nil {
		return state, fmt.Errorf("getting ClusterVersion: %w", err)
	}
	overrides, err := c.setSnapshotControllerOverrides(ctx, version)
	if err != nil {
		return state, err
	}
	state.snapshotControllerOverrides = overrides
	c.logRemap("Configured ClusterVersion snapshot-controller overrides", logrus.Fields{"updated_overrides": snapshotControllerOverrideNames(overrides)})
	for _, workload := range snapshotControllerWorkloads() {
		object, err := c.getDeployment(ctx, workload.namespace, workload.name)
		if err != nil {
			return state, fmt.Errorf("getting %s %s/%s: %w", workload.kind, workload.namespace, workload.name, err)
		}
		replicas, _, _ := unstructured.NestedInt64(object.Object, "spec", "replicas")
		replicaCount, err := int32ReplicaCount(replicas)
		if err != nil {
			return state, fmt.Errorf("getting %s %s/%s replicas: %w", workload.kind, workload.namespace, workload.name, err)
		}
		state.deployments[workload.namespace+"/"+workload.name] = replicaCount
		c.logWorkloadReplicas("Stopping snapshot reconciliation deployment", object, 0)
		if err := c.scaleDeployment(ctx, object, 0); err != nil {
			return state, err
		}
		if err := c.waitForWorkloadReplicas(ctx, workload.kind, workload.namespace, workload.name, 0); err != nil {
			return state, err
		}
	}
	return state, nil
}

func int32ReplicaCount(replicas int64) (int32, error) {
	if replicas < 0 || replicas > math.MaxInt32 {
		return 0, fmt.Errorf("replica count %d is outside the int32 range", replicas)
	}
	return int32(replicas), nil
}

func (c *managementClusterClient) waitForHostedControlPlanePaused(ctx context.Context, namespace, name string) error {
	for {
		hcp, err := c.getHostedControlPlane(ctx, namespace, name)
		if err != nil {
			return err
		}
		pausedUntil, _, _ := unstructured.NestedString(hcp.Object, "spec", "pausedUntil")
		c.logRemap("Waiting for HostedCluster pause to propagate to HostedControlPlane", logrus.Fields{"hcp": name, "namespace": namespace, "paused_until": pausedUntil})
		if pausedUntil == "true" {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for HostedCluster pause to propagate to HostedControlPlane %s/%s: %w", namespace, name, ctx.Err())
		case <-time.After(remapPollInterval):
		}
	}
}

func (c *managementClusterClient) resumeSnapshotControllers(ctx context.Context, state *remapState) error {
	for _, deployment := range snapshotControllerWorkloads() {
		key := deployment.namespace + "/" + deployment.name
		replicas, found := state.deployments[key]
		if !found {
			continue
		}
		object, err := c.getDeployment(ctx, deployment.namespace, deployment.name)
		if err != nil {
			return err
		}
		c.logWorkloadReplicas("Restarting snapshot reconciliation deployment", object, replicas)
		if err := c.scaleDeployment(ctx, object, replicas); err != nil {
			return err
		}
		if err := c.waitForWorkloadReplicas(ctx, "Deployment", deployment.namespace, deployment.name, replicas); err != nil {
			return err
		}
	}
	if err := c.restoreSnapshotControllerOverrides(ctx, state.snapshotControllerOverrides); err != nil {
		return err
	}
	c.logRemap("Restored ClusterVersion snapshot-controller overrides", logrus.Fields{"restored_overrides": snapshotControllerOverrideNames(state.snapshotControllerOverrides)})
	return nil
}

func (c *managementClusterClient) recoverSnapshotReconciliation(ctx context.Context, state *remapState) error {
	var errs []error
	if err := c.resumeSnapshotControllers(ctx, state); err != nil {
		errs = append(errs, fmt.Errorf("restarting snapshot controllers: %w", err))
	}
	if replicas, found := state.deployments[state.hcp.GetNamespace()+"/etcd"]; found {
		etcd, err := c.getStatefulSet(ctx, state.hcp.GetNamespace(), "etcd")
		if err != nil {
			errs = append(errs, fmt.Errorf("getting StatefulSet %s/etcd for recovery: %w", state.hcp.GetNamespace(), err))
		} else if err := c.scaleDeployment(ctx, etcd, replicas); err != nil {
			errs = append(errs, fmt.Errorf("restarting etcd: %w", err))
		} else if err := c.waitForWorkloadReplicas(ctx, "StatefulSet", state.hcp.GetNamespace(), "etcd", replicas); err != nil {
			errs = append(errs, fmt.Errorf("waiting for etcd recovery: %w", err))
		}
	}
	if state.hostedClusterPaused {
		originalPause, found, err := unstructured.NestedString(state.hostedCluster.Object, "spec", "pausedUntil")
		if err != nil {
			errs = append(errs, fmt.Errorf("reading original HostedCluster pause state: %w", err))
		} else if found {
			if err := c.setPaused(ctx, state.hostedCluster, &originalPause); err != nil {
				errs = append(errs, fmt.Errorf("restoring HostedCluster pause state during recovery: %w", err))
			}
		} else if err := c.setPaused(ctx, state.hostedCluster, nil); err != nil {
			errs = append(errs, fmt.Errorf("unpausing HostedCluster during recovery: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (c *managementClusterClient) stopSnapshotControllers(ctx context.Context, state *remapState) error {
	version, err := c.getClusterVersion(ctx)
	if err != nil {
		return fmt.Errorf("getting ClusterVersion before stopping snapshot controllers: %w", err)
	}
	if _, err := c.setSnapshotControllerOverrides(ctx, version); err != nil {
		return fmt.Errorf("setting ClusterVersion snapshot-controller overrides before stopping snapshot controllers: %w", err)
	}
	for _, deployment := range snapshotControllerWorkloads() {
		object, err := c.getDeployment(ctx, deployment.namespace, deployment.name)
		if err != nil {
			return err
		}
		c.logWorkloadReplicas("Stopping snapshot reconciliation deployment", object, 0)
		if err := c.scaleDeployment(ctx, object, 0); err != nil {
			return err
		}
		if err := c.waitForWorkloadReplicas(ctx, "Deployment", deployment.namespace, deployment.name, 0); err != nil {
			return err
		}
	}
	return nil
}

func snapshotControllerWorkloads() []struct{ kind, namespace, name string } {
	return []struct{ kind, namespace, name string }{
		{"Deployment", storageOperatorNamespace, "cluster-storage-operator"},
		{"Deployment", storageOperatorNamespace, "csi-snapshot-controller-operator"},
		{"Deployment", storageOperatorNamespace, "csi-snapshot-controller"},
	}
}

func (c *managementClusterClient) replaceVSC(ctx context.Context, original *unstructured.Unstructured, handle string) error {
	replacement, err := replacementVSC(original, handle)
	if err != nil {
		return fmt.Errorf("building replacement VolumeSnapshotContent: %w", err)
	}
	current, err := c.getVolumeSnapshotContent(ctx, original.GetName())
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err == nil {
		c.logRemap("Removing finalizers from existing VolumeSnapshotContent", logrus.Fields{"volume_snapshot_content": original.GetName()})
		base := current.DeepCopy()
		current.SetFinalizers(nil)
		if err := unstructured.SetNestedField(current.Object, "Retain", "spec", "deletionPolicy"); err != nil {
			return fmt.Errorf("setting Retain deletion policy: %w", err)
		}
		if err := c.client.Patch(ctx, current, client.MergeFrom(base)); err != nil {
			return err
		}
		if err := c.client.Delete(ctx, current); err != nil {
			return err
		}
		c.logRemap("Waiting for existing VolumeSnapshotContent deletion", logrus.Fields{"volume_snapshot_content": original.GetName()})
		for {
			_, err = c.getVolumeSnapshotContent(ctx, original.GetName())
			if apierrors.IsNotFound(err) {
				break
			}
			if err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(remapPollInterval):
			}
		}
	}
	if err := c.client.Create(ctx, replacement); err != nil {
		return err
	}
	return nil
}

func replacementVSC(original *unstructured.Unstructured, handle string) (*unstructured.Unstructured, error) {
	replacement := original.DeepCopy()
	replacement.SetResourceVersion("")
	replacement.SetUID("")
	replacement.SetGeneration(0)
	replacement.SetCreationTimestamp(metav1.Time{})
	replacement.SetDeletionTimestamp(nil)
	replacement.SetDeletionGracePeriodSeconds(nil)
	replacement.SetFinalizers(nil)
	replacement.SetManagedFields(nil)
	unstructured.RemoveNestedField(replacement.Object, "status")
	unstructured.RemoveNestedField(replacement.Object, "spec", "source", "volumeHandle")
	if err := unstructured.SetNestedField(replacement.Object, handle, "spec", "source", "snapshotHandle"); err != nil {
		return nil, fmt.Errorf("setting snapshot handle: %w", err)
	}
	return replacement, nil
}

func (c *managementClusterClient) restoreVSCs(ctx context.Context, originals map[string]*unstructured.Unstructured) error {
	for _, original := range originals {
		if err := c.replaceVSC(ctx, original, snapshotHandle(original)); err != nil {
			return err
		}
	}
	return nil
}

func snapshotHandle(content *unstructured.Unstructured) string {
	handle, _, _ := unstructured.NestedString(content.Object, "status", "snapshotHandle")
	if handle == "" {
		handle, _, _ = unstructured.NestedString(content.Object, "spec", "source", "snapshotHandle")
	}
	return handle
}

func (c *managementClusterClient) waitForRemappedVSCs(ctx context.Context, mappings map[string]string) error {
	for {
		ready := true
		for name, handle := range mappings {
			content, err := c.getVolumeSnapshotContent(ctx, name)
			if err != nil {
				return err
			}
			actual := snapshotHandle(content)
			readyToUse, _, _ := unstructured.NestedBool(content.Object, "status", "readyToUse")
			message, _, _ := unstructured.NestedString(content.Object, "status", "error", "message")
			c.logRemap("Waiting for replacement VolumeSnapshotContent readiness", logrus.Fields{"volume_snapshot_content": name, "expected_snapshot": handle, "actual_snapshot": actual, "ready_to_use": readyToUse, "snapshot_error": message})
			if actual != handle || !readyToUse {
				ready = false
				break
			}
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for replacement VolumeSnapshotContents to become ready: %w", ctx.Err())
		case <-time.After(remapPollInterval):
		}
	}
}

func (c *managementClusterClient) logRemap(message string, fields logrus.Fields) {
	c.cfg.Logger.WithFields(fields).Info(message)
}

func (c *managementClusterClient) verifyRemappedVSCHandles(ctx context.Context, mappings map[string]string) error {
	for name, handle := range mappings {
		content, err := c.getVolumeSnapshotContent(ctx, name)
		if err != nil {
			return err
		}
		if snapshotHandle(content) != handle {
			return fmt.Errorf("VolumeSnapshotContent %s does not reference destination snapshot %s", name, handle)
		}
	}
	return nil
}

func (c *managementClusterClient) setPaused(ctx context.Context, object *unstructured.Unstructured, value *string) error {
	var pausedUntil any
	if value != nil {
		pausedUntil = *value
	}
	patch, err := json.Marshal(map[string]any{"spec": map[string]any{"pausedUntil": pausedUntil}})
	if err != nil {
		return err
	}
	return c.client.Patch(ctx, object.DeepCopy(), client.RawPatch(types.MergePatchType, patch))
}

func (c *managementClusterClient) scaleDeployment(ctx context.Context, object *unstructured.Unstructured, replicas int32) error {
	copy := object.DeepCopy()
	if err := unstructured.SetNestedField(copy.Object, int64(replicas), "spec", "replicas"); err != nil {
		return err
	}
	if err := c.client.Patch(ctx, copy, client.MergeFrom(object)); err != nil {
		return err
	}
	c.logWorkloadReplicas("Patched workload replica target", copy, replicas)
	return nil
}

func (c *managementClusterClient) waitForWorkloadReplicas(ctx context.Context, kind, namespace, name string, replicas int32) error {
	for {
		workload, err := c.getWorkload(ctx, kind, namespace, name)
		if err != nil {
			return err
		}
		c.logWorkloadReplicas("Waiting for workload replicas", workload, replicas)
		observed, _, _ := unstructured.NestedInt64(workload.Object, "status", "replicas")
		if observed == int64(replicas) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for %s %s/%s to scale to %d: %w", kind, namespace, name, replicas, ctx.Err())
		case <-time.After(remapPollInterval):
		}
	}
}

func (c *managementClusterClient) logWorkloadReplicas(message string, workload *unstructured.Unstructured, expected int32) {
	specReplicas, _, _ := unstructured.NestedInt64(workload.Object, "spec", "replicas")
	statusReplicas, _, _ := unstructured.NestedInt64(workload.Object, "status", "replicas")
	readyReplicas, _, _ := unstructured.NestedInt64(workload.Object, "status", "readyReplicas")
	availableReplicas, _, _ := unstructured.NestedInt64(workload.Object, "status", "availableReplicas")
	observedGeneration, _, _ := unstructured.NestedInt64(workload.Object, "status", "observedGeneration")
	c.cfg.Logger.WithFields(logrus.Fields{
		"kind":                workload.GetKind(),
		"namespace":           workload.GetNamespace(),
		"name":                workload.GetName(),
		"expected_replicas":   expected,
		"spec_replicas":       specReplicas,
		"status_replicas":     statusReplicas,
		"ready_replicas":      readyReplicas,
		"available_replicas":  availableReplicas,
		"generation":          workload.GetGeneration(),
		"observed_generation": observedGeneration,
	}).Info(message)
}

func (c *managementClusterClient) setSnapshotControllerOverrides(ctx context.Context, version *unstructured.Unstructured) ([]snapshotControllerOverrideChange, error) {
	changes := make([]snapshotControllerOverrideChange, 0, len(snapshotControllerWorkloads()))
	// ClusterVersion overrides are an atomic list. Re-fetching and retrying the
	// update prevents us from replacing overrides added by another controller.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := version.DeepCopy()
		if err := c.client.Get(ctx, types.NamespacedName{Name: "version"}, current); err != nil {
			return err
		}
		overrides, _, _ := unstructured.NestedSlice(current.Object, "spec", "overrides")
		changes = changes[:0]
		for _, workload := range snapshotControllerWorkloads() {
			fields, found := snapshotControllerOverride(overrides, workload.name)
			if !found {
				overrides = append(overrides, map[string]any{"group": "apps", "kind": "Deployment", "name": workload.name, "namespace": storageOperatorNamespace, "unmanaged": true})
				changes = append(changes, snapshotControllerOverrideChange{name: workload.name, created: true})
				continue
			}
			if fields["unmanaged"] != true {
				original, hadUnmanaged := fields["unmanaged"]
				fields["unmanaged"] = true
				changes = append(changes, snapshotControllerOverrideChange{name: workload.name, hadUnmanaged: hadUnmanaged, originalUnmanaged: original})
			}
		}
		if err := unstructured.SetNestedSlice(current.Object, overrides, "spec", "overrides"); err != nil {
			return err
		}
		return c.client.Update(ctx, current)
	})
	if err != nil {
		return nil, err
	}
	return changes, nil
}

func (c *managementClusterClient) restoreSnapshotControllerOverrides(ctx context.Context, changes []snapshotControllerOverrideChange) error {
	if len(changes) == 0 {
		return nil
	}
	// Remove only overrides added by this operation, while retaining changes
	// made to the atomic overrides list while reconciliation was paused.
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		version := &unstructured.Unstructured{}
		version.SetGroupVersionKind(schema.GroupVersionKind{Group: clusterVersionGroup, Version: "v1", Kind: "ClusterVersion"})
		if err := c.client.Get(ctx, types.NamespacedName{Name: "version"}, version); err != nil {
			return err
		}
		overrides, _, _ := unstructured.NestedSlice(version.Object, "spec", "overrides")
		filtered := overrides[:0]
		for _, override := range overrides {
			remove := false
			for _, change := range changes {
				fields, matches := snapshotControllerOverride([]any{override}, change.name)
				if !matches {
					continue
				}
				if change.created && fields["unmanaged"] == true {
					remove = true
					break
				}
				if !change.created {
					if change.hadUnmanaged {
						fields["unmanaged"] = change.originalUnmanaged
					} else {
						delete(fields, "unmanaged")
					}
				}
			}
			if !remove {
				filtered = append(filtered, override)
			}
		}
		if err := unstructured.SetNestedSlice(version.Object, filtered, "spec", "overrides"); err != nil {
			return err
		}
		return c.client.Update(ctx, version)
	})
}

func snapshotControllerOverrideNames(changes []snapshotControllerOverrideChange) []string {
	names := make([]string, len(changes))
	for i, change := range changes {
		names[i] = change.name
	}
	return names
}

func hasUnmanagedOverride(overrides []any, name string) bool {
	fields, found := snapshotControllerOverride(overrides, name)
	return found && fields["unmanaged"] == true
}

func snapshotControllerOverride(overrides []any, name string) (map[string]any, bool) {
	for _, override := range overrides {
		fields, ok := override.(map[string]any)
		if ok && fields["group"] == "apps" && fields["kind"] == "Deployment" && fields["name"] == name && fields["namespace"] == storageOperatorNamespace {
			return fields, true
		}
	}
	return nil, false
}

func (c *managementClusterClient) getObject(ctx context.Context, group, version, kind, namespace, name string) (*unstructured.Unstructured, error) {
	object := &unstructured.Unstructured{}
	object.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: version, Kind: kind})
	if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, object); err != nil {
		return nil, err
	}
	return object, nil
}

func (c *managementClusterClient) getHostedControlPlane(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	return c.getObject(ctx, hypershiftGroup, "v1beta1", "HostedControlPlane", namespace, name)
}

func (c *managementClusterClient) getStatefulSet(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	return c.getWorkload(ctx, "StatefulSet", namespace, name)
}

func (c *managementClusterClient) getDeployment(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	return c.getWorkload(ctx, "Deployment", namespace, name)
}

func (c *managementClusterClient) getWorkload(ctx context.Context, kind, namespace, name string) (*unstructured.Unstructured, error) {
	return c.getObject(ctx, "apps", "v1", kind, namespace, name)
}

func (c *managementClusterClient) getClusterVersion(ctx context.Context) (*unstructured.Unstructured, error) {
	return c.getObject(ctx, clusterVersionGroup, "v1", "ClusterVersion", "", "version")
}

func (c *managementClusterClient) getVolumeSnapshotContent(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return c.getObject(ctx, "snapshot.storage.k8s.io", "v1", "VolumeSnapshotContent", "", name)
}

func (c *managementClusterClient) findObject(ctx context.Context, group, version, kind, name string) (*unstructured.Unstructured, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: version, Kind: kind + "List"})
	if err := c.client.List(ctx, list); err != nil {
		return nil, err
	}
	var found *unstructured.Unstructured
	for i := range list.Items {
		if list.Items[i].GetName() == name {
			if found != nil {
				return nil, fmt.Errorf("multiple %s named %s", kind, name)
			}
			found = list.Items[i].DeepCopy()
		}
	}
	if found == nil {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: group, Resource: kind}, name)
	}
	return found, nil
}

func (c *managementClusterClient) findHostedCluster(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return c.findObject(ctx, hypershiftGroup, "v1beta1", "HostedCluster", name)
}

func ptr(value string) *string { return &value }

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

func veleroObject(resources schema.GroupVersionKind, namespace, kind, name string) *unstructured.Unstructured {
	object := &unstructured.Unstructured{}
	object.SetGroupVersionKind(schema.GroupVersionKind{Group: resources.Group, Version: resources.Version, Kind: kind})
	object.SetNamespace(namespace)
	object.SetName(name)
	return object
}

func veleroList(resources schema.GroupVersionKind, kind string) *unstructured.UnstructuredList {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{Group: resources.Group, Version: resources.Version, Kind: kind + "List"})
	return list
}

func (c *managementClusterClient) verifyBackupStorageLocation(ctx context.Context, resources schema.GroupVersionKind) error {
	namespace := c.cfg.VeleroNamespace
	locations := veleroList(resources, "BackupStorageLocation")
	if err := c.client.List(ctx, locations, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("listing BackupStorageLocations: %w", err)
	}
	for _, location := range locations.Items {
		phase, _, _ := unstructured.NestedString(location.Object, "status", "phase")
		if phase == "Available" {
			return nil
		}
	}
	return fmt.Errorf("no available BackupStorageLocation in namespace %s", namespace)
}

func (c *managementClusterClient) verifyBackup(ctx context.Context, resources schema.GroupVersionKind, backupName string) error {
	namespace := c.cfg.VeleroNamespace
	backup := veleroObject(resources, namespace, "Backup", backupName)
	if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: backupName}, backup); err != nil {
		return fmt.Errorf("getting Velero Backup %s: %w", backupName, err)
	}
	phase, _, _ := unstructured.NestedString(backup.Object, "status", "phase")
	if phase != "Completed" {
		return fmt.Errorf("velero Backup %s is %q, expected Completed", backupName, phase)
	}
	return nil
}

func (c *managementClusterClient) getOrCreateRestore(ctx context.Context, resources schema.GroupVersionKind, clusterID, backupName string) (*unstructured.Unstructured, bool, error) {
	namespace := c.cfg.VeleroNamespace
	restores := veleroList(resources, "Restore")
	if err := c.client.List(ctx, restores, client.InNamespace(namespace), client.MatchingLabels{restoreManagedLabel: restoreManagedValue}); err != nil {
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

	restore := newVeleroRestore(resources, namespace, clusterID, backupName, maxAttempt+1)
	if err := c.client.Create(ctx, restore); err != nil {
		return nil, false, fmt.Errorf("creating Velero Restore %s for backup %s: %w", restore.GetName(), backupName, err)
	}
	return restore, true, nil
}

func (c *managementClusterClient) waitForRestore(ctx context.Context, resources schema.GroupVersionKind, restoreName, backupName string) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		namespace := c.cfg.VeleroNamespace
		restore := veleroObject(resources, namespace, "Restore", restoreName)
		if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restoreName}, restore); err != nil {
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

func newVeleroRestore(resources schema.GroupVersionKind, namespace, clusterID, backupName string, attempt int) *unstructured.Unstructured {
	sum := sha256.Sum256([]byte(clusterID + "\x00" + backupName))
	name := fmt.Sprintf("osdctl-restore-%x-%d", sum[:6], attempt)
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": resources.GroupVersion().String(), "kind": "Restore",
		"metadata": map[string]any{"name": name, "namespace": namespace, "labels": map[string]any{restoreManagedLabel: restoreManagedValue}, "annotations": map[string]any{restoreBackupAnnotation: backupName, restoreClusterAnnotation: clusterID, restoreAttemptAnnotation: strconv.Itoa(attempt)}},
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
