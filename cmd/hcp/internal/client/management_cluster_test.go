package client

import (
	"bytes"
	"context"
	"math"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestTerminalRestorePhase(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		phase    string
		terminal bool
	}{
		{phase: "Completed"},
		{phase: "InProgress"},
		{phase: "WaitingForPluginOperationsPartiallyFailed"},
		{phase: "FinalizingPartiallyFailed"},
		{phase: "Failed", terminal: true},
		{phase: "FailedValidation", terminal: true},
		{phase: "PartiallyFailed", terminal: true},
	} {
		t.Run(tt.phase, func(t *testing.T) {
			t.Parallel()
			if got := terminalRestorePhase(tt.phase); got != tt.terminal {
				t.Errorf("terminalRestorePhase(%q) = %t, want %t", tt.phase, got, tt.terminal)
			}
		})
	}
}

func TestManagementClusterClientGetVeleroSchedule(t *testing.T) {
	schedule := &unstructured.Unstructured{}
	schedule.SetGroupVersionKind(schema.GroupVersionKind{Group: "velero.io", Version: "v1", Kind: "Schedule"})
	schedule.SetNamespace("openshift-adp")
	schedule.SetName("cluster-daily")
	client := &managementClusterClient{client: fake.NewClientBuilder().WithObjects(schedule).Build()}

	got, err := client.GetVeleroSchedule(context.Background(), "openshift-adp", "cluster-daily")
	require.NoError(t, err)
	assert.Equal(t, schedule.GetName(), got.GetName())

	_, err = client.GetVeleroSchedule(context.Background(), "openshift-adp", "missing")
	assert.True(t, apierrors.IsNotFound(err))
	assert.ErrorContains(t, err, "openshift-adp")
}

func TestManagementClusterClientResourceLookups(t *testing.T) {
	objects := []ctrlclient.Object{
		&unstructured.Unstructured{Object: map[string]any{"apiVersion": hypershiftGroup + "/v1beta1", "kind": "HostedControlPlane", "metadata": map[string]any{"name": "example", "namespace": "control-plane"}}},
		&unstructured.Unstructured{Object: map[string]any{"apiVersion": hypershiftGroup + "/v1beta1", "kind": "HostedCluster", "metadata": map[string]any{"name": "example", "namespace": "clusters"}}},
		&unstructured.Unstructured{Object: map[string]any{"apiVersion": "apps/v1", "kind": "StatefulSet", "metadata": map[string]any{"name": "etcd", "namespace": "control-plane"}}},
		&unstructured.Unstructured{Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "snapshot-controller", "namespace": storageOperatorNamespace}}},
		&unstructured.Unstructured{Object: map[string]any{"apiVersion": clusterVersionGroup + "/v1", "kind": "ClusterVersion", "metadata": map[string]any{"name": "version"}}},
		&unstructured.Unstructured{Object: map[string]any{"apiVersion": "snapshot.storage.k8s.io/v1", "kind": "VolumeSnapshotContent", "metadata": map[string]any{"name": "content"}}},
	}
	managementClient := NewManagementClusterClient(fake.NewClientBuilder().WithObjects(objects...).Build(), nil, nil)

	for _, tt := range []struct {
		name       string
		get        func() (*unstructured.Unstructured, error)
		gvk        schema.GroupVersionKind
		namespace  string
		objectName string
	}{
		{
			name: "HostedControlPlane",
			get: func() (*unstructured.Unstructured, error) {
				return managementClient.getHostedControlPlane(context.Background(), "control-plane", "example")
			},
			gvk:        schema.GroupVersionKind{Group: hypershiftGroup, Version: "v1beta1", Kind: "HostedControlPlane"},
			namespace:  "control-plane",
			objectName: "example",
		},
		{
			name: "HostedCluster",
			get: func() (*unstructured.Unstructured, error) {
				return managementClient.findHostedCluster(context.Background(), "example")
			},
			gvk:        schema.GroupVersionKind{Group: hypershiftGroup, Version: "v1beta1", Kind: "HostedCluster"},
			namespace:  "clusters",
			objectName: "example",
		},
		{
			name: "StatefulSet",
			get: func() (*unstructured.Unstructured, error) {
				return managementClient.getStatefulSet(context.Background(), "control-plane", "etcd")
			},
			gvk:        schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"},
			namespace:  "control-plane",
			objectName: "etcd",
		},
		{
			name: "Deployment",
			get: func() (*unstructured.Unstructured, error) {
				return managementClient.getDeployment(context.Background(), storageOperatorNamespace, "snapshot-controller")
			},
			gvk:        schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			namespace:  storageOperatorNamespace,
			objectName: "snapshot-controller",
		},
		{
			name: "ClusterVersion",
			get: func() (*unstructured.Unstructured, error) {
				return managementClient.getClusterVersion(context.Background())
			},
			gvk:        schema.GroupVersionKind{Group: clusterVersionGroup, Version: "v1", Kind: "ClusterVersion"},
			objectName: "version",
		},
		{
			name: "VolumeSnapshotContent",
			get: func() (*unstructured.Unstructured, error) {
				return managementClient.getVolumeSnapshotContent(context.Background(), "content")
			},
			gvk:        schema.GroupVersionKind{Group: "snapshot.storage.k8s.io", Version: "v1", Kind: "VolumeSnapshotContent"},
			objectName: "content",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.get()
			require.NoError(t, err)
			assert.Equal(t, tt.gvk, got.GroupVersionKind())
			assert.Equal(t, tt.namespace, got.GetNamespace())
			assert.Equal(t, tt.objectName, got.GetName())
		})
	}
}

func TestReadyPodName(t *testing.T) {
	readyPod := func(name string) corev1.Pod {
		return corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "openshift-adp"}, Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	}
	terminatingPod := readyPod("terminating")
	now := metav1.Now()
	terminatingPod.DeletionTimestamp = &now

	for _, tt := range []struct {
		name string
		pods []corev1.Pod
		want string
	}{
		{name: "first ready running pod", pods: []corev1.Pod{readyPod("first"), readyPod("second")}, want: "first"},
		{name: "terminating pod skipped", pods: []corev1.Pod{terminatingPod, readyPod("ready")}, want: "ready"},
		{name: "pending pod skipped", pods: []corev1.Pod{{Status: corev1.PodStatus{Phase: corev1.PodPending}}}},
		{name: "not ready pod skipped", pods: []corev1.Pod{{Status: corev1.PodStatus{Phase: corev1.PodRunning}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			name, err := readyPodName(tt.pods, "openshift-adp", "app", "velero")
			if tt.want == "" {
				assert.ErrorContains(t, err, "no running Velero pod")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, name)
		})
	}
}

func TestJoinSortedMap(t *testing.T) {
	for _, tt := range []struct {
		name   string
		values map[string]string
		want   string
	}{
		{name: "empty", values: map[string]string{}, want: ""},
		{name: "single value", values: map[string]string{"incident": "OHSS-1234"}, want: "incident=OHSS-1234"},
		{name: "sorted values", values: map[string]string{"zzz": "last", "aaa": "first", "mmm": "middle"}, want: "aaa=first,mmm=middle,zzz=last"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, joinSortedMap(tt.values))
		})
	}
}

func TestReplacementVSCPreservesBindingAndLabels(t *testing.T) {
	original := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "etcd", "uid": "old", "resourceVersion": "7", "labels": map[string]any{"velero.io/backup-name": "backup", "velero.io/restore-name": "restore"}},
		"spec":     map[string]any{"driver": "ebs.csi.aws.com", "volumeSnapshotRef": map[string]any{"name": "etcd", "namespace": "hcp"}, "source": map[string]any{"snapshotHandle": "snap-source"}},
		"status":   map[string]any{"readyToUse": true, "snapshotHandle": "snap-source"},
	}}

	replacement, err := replacementVSC(original, "snap-destination")
	require.NoError(t, err)

	handle, _, _ := unstructured.NestedString(replacement.Object, "spec", "source", "snapshotHandle")
	assert.Equal(t, "snap-destination", handle)
	assert.Empty(t, replacement.GetResourceVersion())
	assert.Empty(t, string(replacement.GetUID()))
	assert.Equal(t, "backup", replacement.GetLabels()["velero.io/backup-name"])
	_, found, _ := unstructured.NestedMap(replacement.Object, "status")
	assert.False(t, found)
}

func TestRemapOriginalsRequiresEveryMappedEBSVSC(t *testing.T) {
	contents := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
		{Object: map[string]any{
			"metadata": map[string]any{"name": "etcd"},
			"spec": map[string]any{
				"driver":            "ebs.csi.aws.com",
				"volumeSnapshotRef": map[string]any{"namespace": "hcp"},
			},
		}},
	}}

	originals, err := remapOriginals(contents, "hcp", map[string]string{"etcd": "snap-destination"})

	require.NoError(t, err)
	assert.Contains(t, originals, "etcd")
	_, err = remapOriginals(contents, "hcp", map[string]string{"missing": "snap-destination"})
	assert.ErrorContains(t, err, "not all mapped")
}

func TestSetPausedPatchesStaleObject(t *testing.T) {
	hostedCluster := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "hypershift.openshift.io/v1beta1",
		"kind":       "HostedCluster",
		"metadata":   map[string]any{"name": "example", "namespace": "clusters"},
		"spec":       map[string]any{"release": map[string]any{"image": "original"}},
	}}
	kubeClient := fake.NewClientBuilder().WithObjects(hostedCluster).Build()
	managementClient := NewManagementClusterClient(kubeClient, nil, nil)
	stale := hostedCluster.DeepCopy()

	current := hostedCluster.DeepCopy()
	key := types.NamespacedName{Name: current.GetName(), Namespace: current.GetNamespace()}
	require.NoError(t, kubeClient.Get(context.Background(), key, current))
	current.SetLabels(map[string]string{"concurrent": "change"})
	require.NoError(t, kubeClient.Update(context.Background(), current))

	require.NoError(t, managementClient.setPaused(context.Background(), stale, ptr("true")))
	require.NoError(t, managementClient.setPaused(context.Background(), stale, nil))

	updated := hostedCluster.DeepCopy()
	require.NoError(t, kubeClient.Get(context.Background(), key, updated))
	pausedUntil, found, err := unstructured.NestedString(updated.Object, "spec", "pausedUntil")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, pausedUntil)
	assert.Equal(t, "change", updated.GetLabels()["concurrent"])
}

func TestPauseSnapshotReconciliationReturnsStateForRecovery(t *testing.T) {
	for _, tt := range []struct {
		name          string
		originalPause string
		found         bool
	}{
		{name: "unpaused"},
		{name: "already paused", originalPause: "2030-01-01T00:00:00Z", found: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hcp := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "hypershift.openshift.io/v1beta1",
				"kind":       "HostedControlPlane",
				"metadata":   map[string]any{"name": "example", "namespace": "hcp"},
				"spec":       map[string]any{"pausedUntil": "true"},
			}}
			hostedClusterSpec := map[string]any{}
			if tt.found {
				hostedClusterSpec["pausedUntil"] = tt.originalPause
			}
			hostedCluster := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "hypershift.openshift.io/v1beta1",
				"kind":       "HostedCluster",
				"metadata":   map[string]any{"name": "example", "namespace": "clusters"},
				"spec":       hostedClusterSpec,
			}}
			kubeClient := fake.NewClientBuilder().WithObjects(hcp, hostedCluster).Build()
			managementClient := NewManagementClusterClient(kubeClient, nil, nil)

			state, err := managementClient.pauseSnapshotReconciliation(context.Background(), "hcp", "example")

			require.ErrorContains(t, err, "getting StatefulSet hcp/etcd")
			require.NotNil(t, state)
			assert.True(t, state.hostedClusterPaused)
			require.NoError(t, managementClient.recoverSnapshotReconciliation(context.Background(), state))

			updated := hostedCluster.DeepCopy()
			require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Name: "example", Namespace: "clusters"}, updated))
			pausedUntil, found, err := unstructured.NestedString(updated.Object, "spec", "pausedUntil")
			require.NoError(t, err)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.originalPause, pausedUntil)
		})
	}
}

func TestReplaceVSCLeavesOriginalWhenReplacementIsInvalid(t *testing.T) {
	original := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "snapshot.storage.k8s.io/v1",
		"kind":       "VolumeSnapshotContent",
		"metadata":   map[string]any{"name": "etcd"},
		"spec":       map[string]any{"source": "invalid"},
	}}
	kubeClient := fake.NewClientBuilder().WithObjects(original).Build()
	managementClient := NewManagementClusterClient(kubeClient, nil, nil)

	err := managementClient.replaceVSC(context.Background(), original, "snap-destination")

	require.ErrorContains(t, err, "building replacement VolumeSnapshotContent")
	current := original.DeepCopy()
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Name: "etcd"}, current))
	assert.Equal(t, "invalid", current.Object["spec"].(map[string]any)["source"])
}

func TestReplaceVSCRetainsExistingSnapshotBeforeDeletion(t *testing.T) {
	original := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "snapshot.storage.k8s.io/v1",
		"kind":       "VolumeSnapshotContent",
		"metadata":   map[string]any{"name": "etcd", "finalizers": []any{"snapshot.storage.kubernetes.io/volumesnapshotcontent-bound-protection"}},
		"spec":       map[string]any{"deletionPolicy": "Delete", "source": map[string]any{"snapshotHandle": "snap-source"}},
	}}
	var patched *unstructured.Unstructured
	kubeClient := fake.NewClientBuilder().WithObjects(original).Build()
	managementClient := NewManagementClusterClient(interceptor.NewClient(kubeClient, interceptor.Funcs{
		Patch: func(ctx context.Context, client ctrlclient.WithWatch, object ctrlclient.Object, patch ctrlclient.Patch, opts ...ctrlclient.PatchOption) error {
			patched = object.(*unstructured.Unstructured).DeepCopy()
			return client.Patch(ctx, object, patch, opts...)
		},
	}), nil, nil)

	require.NoError(t, managementClient.replaceVSC(context.Background(), original, "snap-destination"))
	require.NotNil(t, patched)
	deletionPolicy, found, err := unstructured.NestedString(patched.Object, "spec", "deletionPolicy")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "Retain", deletionPolicy)
	assert.Empty(t, patched.GetFinalizers())
}

func TestInt32ReplicaCount(t *testing.T) {
	for _, tt := range []struct {
		name     string
		replicas int64
		want     int32
		err      string
	}{
		{name: "zero", replicas: 0},
		{name: "maximum", replicas: math.MaxInt32, want: math.MaxInt32},
		{name: "negative", replicas: -1, err: "outside the int32 range"},
		{name: "overflow", replicas: math.MaxInt32 + 1, err: "outside the int32 range"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := int32ReplicaCount(tt.replicas)
			if tt.err != "" {
				assert.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWaitForHostedControlPlanePaused(t *testing.T) {
	hcp := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "hypershift.openshift.io/v1beta1",
		"kind":       "HostedControlPlane",
		"metadata":   map[string]any{"name": "example", "namespace": "hcp"},
		"spec":       map[string]any{"pausedUntil": "true"},
	}}
	managementClient := NewManagementClusterClient(fake.NewClientBuilder().WithObjects(hcp).Build(), nil, nil)

	require.NoError(t, managementClient.waitForHostedControlPlanePaused(context.Background(), "hcp", "example"))
}

func TestSnapshotControllerWorkloads(t *testing.T) {
	workloads := snapshotControllerWorkloads()

	require.Len(t, workloads, 3)
	assert.Equal(t, "cluster-storage-operator", workloads[0].name)
	assert.Equal(t, "csi-snapshot-controller-operator", workloads[1].name)
	assert.Equal(t, "csi-snapshot-controller", workloads[2].name)
	for _, workload := range workloads {
		assert.Equal(t, storageOperatorNamespace, workload.namespace)
	}
}

func TestSnapshotControllerOverridesRestoreExistingValues(t *testing.T) {
	version := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "config.openshift.io/v1",
		"kind":       "ClusterVersion",
		"metadata":   map[string]any{"name": "version"},
		"spec": map[string]any{"overrides": []any{
			map[string]any{"group": "apps", "kind": "Deployment", "name": "csi-snapshot-controller-operator", "namespace": storageOperatorNamespace, "unmanaged": false},
			map[string]any{"group": "apps", "kind": "Deployment", "name": "unrelated", "namespace": storageOperatorNamespace, "unmanaged": false},
		}},
	}}
	kubeClient := fake.NewClientBuilder().WithObjects(version).Build()
	managementClient := NewManagementClusterClient(kubeClient, nil, nil)

	changes, err := managementClient.setSnapshotControllerOverrides(context.Background(), version)
	require.NoError(t, err)
	assert.Equal(t, []string{"cluster-storage-operator", "csi-snapshot-controller-operator", "csi-snapshot-controller"}, snapshotControllerOverrideNames(changes))

	updated := version.DeepCopy()
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Name: "version"}, updated))
	overrides, found, err := unstructured.NestedSlice(updated.Object, "spec", "overrides")
	require.NoError(t, err)
	require.True(t, found)
	for _, workload := range snapshotControllerWorkloads() {
		assert.True(t, hasUnmanagedOverride(overrides, workload.name))
	}
	assert.False(t, hasUnmanagedOverride(overrides, "unrelated"))

	require.NoError(t, managementClient.restoreSnapshotControllerOverrides(context.Background(), changes))
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Name: "version"}, updated))
	overrides, found, err = unstructured.NestedSlice(updated.Object, "spec", "overrides")
	require.NoError(t, err)
	require.True(t, found)
	assert.False(t, hasUnmanagedOverride(overrides, "cluster-storage-operator"))
	assert.False(t, hasUnmanagedOverride(overrides, "csi-snapshot-controller-operator"))
	assert.False(t, hasUnmanagedOverride(overrides, "csi-snapshot-controller"))

	existing, found := snapshotControllerOverride(overrides, "csi-snapshot-controller-operator")
	require.True(t, found)
	assert.Equal(t, false, existing["unmanaged"])
	_, found = snapshotControllerOverride(overrides, "unrelated")
	assert.True(t, found)
}

func TestManagementClusterClientConfig(t *testing.T) {
	logger := logrus.New()
	var config ManagementClusterClientConfig
	config.Options(WithVeleroNamespace("custom"), WithLogger{Logger: logger})
	config.Defaults()

	assert.Equal(t, "custom", config.VeleroNamespace)
	assert.Same(t, logger, config.Logger)
}

func TestLogWorkloadReplicas(t *testing.T) {
	var output bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&output)
	logger.SetFormatter(&logrus.JSONFormatter{})
	workload := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "snapshot-controller", "namespace": "hcp", "generation": int64(3)},
		"spec":       map[string]any{"replicas": int64(0)},
		"status":     map[string]any{"replicas": int64(1), "readyReplicas": int64(1), "availableReplicas": int64(1), "observedGeneration": int64(2)},
	}}
	managementClient := NewManagementClusterClient(nil, nil, nil, WithLogger{Logger: logger})
	managementClient.logWorkloadReplicas("Waiting for workload replicas", workload, 0)
	managementClient.logRemap("Validated destination snapshot mappings", logrus.Fields{"vsc_count": 2})

	assert.Contains(t, output.String(), `"expected_replicas":0`)
	assert.Contains(t, output.String(), `"status_replicas":1`)
	assert.Contains(t, output.String(), `"observed_generation":2`)
	assert.Contains(t, output.String(), `"vsc_count":2`)
}
