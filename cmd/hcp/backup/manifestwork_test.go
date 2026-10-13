package backup

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestManifestWorkCompareComparesManagementClusters(t *testing.T) {
	sourceOnly := workWithManifests("source-mc", "a-work", "cluster", nil)
	sourceOnly.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue}}
	replacementOnly := workWithManifests("replacement-mc", "z-work", "cluster", nil)
	replacementOnly.Status.Conditions = []metav1.Condition{{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue}}
	unrelated := workWithManifests("source-mc", "other-work", "other-cluster", nil)
	serviceClient := newManifestWorkTestClient(t, sourceOnly, replacementOnly, unrelated)
	builder := &staticKubeClientBuilder{privilegedClient: serviceClient}
	output := &strings.Builder{}
	runner := testManifestWorkRunner(serviceClient, builder, output, "yes\n")
	var resolvedClusterID string
	runner.resolveServiceCluster = func(_ context.Context, clusterID string) (ServiceClusterInfo, error) {
		resolvedClusterID = clusterID
		return ServiceClusterInfo{ID: "service-cluster", Name: "service"}, nil
	}

	err := runner.compare(context.Background(), manifestWorkCompareFlags{clusterID: "hcp", replacementMC: "replacement-mc", reason: "OHSS-1234"})

	require.NoError(t, err, "expected ManifestWork comparison to succeed")
	require.Len(t, builder.calls, 1, "expected one service-cluster connection")
	assert.True(t, builder.calls[0].elevated, "expected comparison to use elevated access")
	assert.Equal(t, "OHSS-1234", builder.calls[0].elevationReason, "expected elevation reason to be forwarded")
	assert.Equal(t, "cluster", resolvedClusterID, "expected canonical HCP ID for service-cluster resolution")
	assert.Contains(t, output.String(), "SOURCE APPLIED", "expected source Applied status column")
	assert.Contains(t, output.String(), "REPLACEMENT AVAILABLE", "expected replacement Available status column")
	assert.Less(t, strings.Index(output.String(), "a-work"), strings.Index(output.String(), "z-work"), "expected comparison output sorted by ManifestWork name")
	assert.Contains(t, output.String(), "a-work  Yes     True", "expected source-only work status")
	assert.Contains(t, output.String(), "z-work  No      -", "expected replacement-only work status")
	assert.NotContains(t, output.String(), "other-work", "expected unrelated cluster work to be excluded")
}

func TestManifestWorkMoveVeleroResourcesCopiesExactWorks(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	secret := drSecretWork("source-mc", secretName, clusterID)
	secret.Finalizers = []string{"work-finalizer"}
	schedules := drSchedulesWork("source-mc", schedulesName, clusterID)
	serviceClient := newManifestWorkTestClient(t, secret, schedules)
	builder := &staticKubeClientBuilder{privilegedClient: serviceClient}
	runner := testManifestWorkRunner(serviceClient, builder, &strings.Builder{}, "yes\n")

	err := runner.moveVeleroResources(context.Background(), manifestWorkMoveFlags{clusterID: clusterID, replacementMC: "replacement-mc", reason: "OHSS-1234"})

	require.NoError(t, err, "expected Velero resources to move")
	require.Len(t, builder.calls, 1, "expected one service-cluster connection")
	assert.True(t, builder.calls[0].elevated, "expected move to use elevated access")
	assert.Equal(t, "OHSS-1234", builder.calls[0].elevationReason, "expected elevation reason to be forwarded")
	destination := &workv1.ManifestWork{}
	require.NoError(t, serviceClient.Get(context.Background(), types.NamespacedName{Namespace: "replacement-mc", Name: secretName}, destination), "expected destination Secret ManifestWork")
	assert.Equal(t, "replacement-mc", destination.Labels[managementClusterLabel], "expected destination management-cluster label")
	assert.Empty(t, destination.Finalizers, "expected copied ManifestWork finalizers to be removed")
	retained := &workv1.ManifestWork{}
	require.NoError(t, serviceClient.Get(context.Background(), types.NamespacedName{Namespace: "source-mc", Name: secretName}, retained), "expected source Secret ManifestWork to remain")
	assert.Equal(t, []string{"work-finalizer"}, retained.Finalizers, "expected source finalizers to be retained")
}

func TestManifestWorkMoveClusterResourcesRequiresVeleroPhase(t *testing.T) {
	serviceClient := newManifestWorkTestClient(t)
	runner := testManifestWorkRunner(serviceClient, &staticKubeClientBuilder{privilegedClient: serviceClient}, &strings.Builder{}, "yes\n")

	err := runner.moveClusterResources(context.Background(), manifestWorkMoveFlags{clusterID: "cluster", replacementMC: "replacement-mc", reason: "OHSS-1234"})

	require.ErrorContains(t, err, "Velero resource ManifestWork", "expected missing Velero prerequisite to stop the cluster move")
}

func TestManifestWorkMoveClusterResourcesPausesAndCopiesTargetWorks(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	works := []*workv1.ManifestWork{
		drSecretWork("replacement-mc", secretName, clusterID),
		drSchedulesWork("replacement-mc", schedulesName, clusterID),
		namespaceWork("source-mc", clusterID),
		hostedClusterWork("source-mc", clusterID),
		nodePoolWork("source-mc", clusterID+"-np-a", clusterID, nodePoolOCMLabel),
		nodePoolWork("source-mc", clusterID+"-np-b", clusterID, nodePoolLabel),
		nodePoolWork("source-mc", "other-np", "other", nodePoolOCMLabel),
	}
	serviceClient := newManifestWorkTestClient(t, works...)
	runner := testManifestWorkRunner(serviceClient, &staticKubeClientBuilder{privilegedClient: serviceClient}, &strings.Builder{}, "yes\n")

	err := runner.moveClusterResources(context.Background(), manifestWorkMoveFlags{clusterID: clusterID, replacementMC: "replacement-mc", reason: "OHSS-1234"})

	require.NoError(t, err, "expected cluster resources to move")
	for _, name := range []string{clusterID + namespaceManifestWorkTail, clusterID, clusterID + "-np-a", clusterID + "-np-b"} {
		destination := &workv1.ManifestWork{}
		require.NoError(t, serviceClient.Get(context.Background(), types.NamespacedName{Namespace: "replacement-mc", Name: name}, destination), "expected destination ManifestWork %s", name)
		assert.Equal(t, "replacement-mc", destination.Labels[managementClusterLabel], "expected destination management-cluster label for %s", name)
	}
	assertPaused(t, serviceClient, "source-mc", clusterID, hostedClusterKind)
	assertPaused(t, serviceClient, "source-mc", clusterID+"-np-a", nodePoolKind)
	assertPaused(t, serviceClient, "replacement-mc", clusterID, hostedClusterKind)
	assertPaused(t, serviceClient, "replacement-mc", clusterID+"-np-b", nodePoolKind)
	assertManifestWorkNotFound(t, serviceClient, "replacement-mc", "other-np")
}

func TestManifestWorkMoveClusterResourcesDeclineDoesNotPauseSource(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	serviceClient := newManifestWorkTestClient(t,
		drSecretWork("replacement-mc", secretName, clusterID),
		drSchedulesWork("replacement-mc", schedulesName, clusterID),
		namespaceWork("source-mc", clusterID),
		hostedClusterWork("source-mc", clusterID),
	)
	runner := testManifestWorkRunner(serviceClient, &staticKubeClientBuilder{privilegedClient: serviceClient}, &strings.Builder{}, "n\n")

	err := runner.moveClusterResources(context.Background(), manifestWorkMoveFlags{clusterID: clusterID, replacementMC: "replacement-mc", reason: "OHSS-1234"})

	require.NoError(t, err, "expected declined cluster move to return without error")
	assertNotPaused(t, serviceClient, "source-mc", clusterID, hostedClusterKind)
	assertManifestWorkNotFound(t, serviceClient, "replacement-mc", clusterID)
}

func TestManifestWorkMoveClusterResourcesRejectsConflictBeforePausingSource(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	conflictingHostedCluster := hostedClusterWork("replacement-mc", clusterID)
	serviceClient := newManifestWorkTestClient(t,
		drSecretWork("replacement-mc", secretName, clusterID),
		drSchedulesWork("replacement-mc", schedulesName, clusterID),
		namespaceWork("source-mc", clusterID),
		hostedClusterWork("source-mc", clusterID),
		conflictingHostedCluster,
	)
	runner := testManifestWorkRunner(serviceClient, &staticKubeClientBuilder{privilegedClient: serviceClient}, &strings.Builder{}, "yes\n")

	err := runner.moveClusterResources(context.Background(), manifestWorkMoveFlags{clusterID: clusterID, replacementMC: "replacement-mc", reason: "OHSS-1234"})

	require.ErrorContains(t, err, "different content", "expected conflicting destination to stop the move")
	assertNotPaused(t, serviceClient, "source-mc", clusterID, hostedClusterKind)
}

func TestManifestWorkMoveClusterResourcesAcceptsPausedDestination(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	sourceHostedCluster := hostedClusterWork("source-mc", clusterID)
	destinationHostedCluster := hostedClusterWork("replacement-mc", clusterID)
	require.NoError(t, pauseManifestWorkPayload(destinationHostedCluster), "failed to prepare paused destination HostedCluster work")
	serviceClient := newManifestWorkTestClient(t,
		drSecretWork("replacement-mc", secretName, clusterID),
		drSchedulesWork("replacement-mc", schedulesName, clusterID),
		namespaceWork("source-mc", clusterID),
		sourceHostedCluster,
		destinationHostedCluster,
	)
	runner := testManifestWorkRunner(serviceClient, &staticKubeClientBuilder{privilegedClient: serviceClient}, &strings.Builder{}, "yes\n")

	err := runner.moveClusterResources(context.Background(), manifestWorkMoveFlags{clusterID: clusterID, replacementMC: "replacement-mc", reason: "OHSS-1234"})

	require.NoError(t, err, "expected existing paused destination to be accepted")
	assertPaused(t, serviceClient, "source-mc", clusterID, hostedClusterKind)
}

func TestManifestWorkMoveRejectsConflictingDestination(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	conflict := drSecretWork("replacement-mc", secretName, clusterID)
	conflict.Spec.Workload.Manifests = nil
	serviceClient := newManifestWorkTestClient(t, drSecretWork("source-mc", secretName, clusterID), drSchedulesWork("source-mc", schedulesName, clusterID), conflict)
	runner := testManifestWorkRunner(serviceClient, &staticKubeClientBuilder{privilegedClient: serviceClient}, &strings.Builder{}, "yes\n")

	err := runner.moveVeleroResources(context.Background(), manifestWorkMoveFlags{clusterID: clusterID, replacementMC: "replacement-mc", reason: "OHSS-1234"})

	require.ErrorContains(t, err, "different content", "expected conflicting destination to be rejected")
}

func TestManifestWorkMoveUsesHCPProvisionShardServiceCluster(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	serviceClient := newManifestWorkTestClient(t, drSecretWork("source-mc", secretName, clusterID), drSchedulesWork("source-mc", schedulesName, clusterID))
	runner := testManifestWorkRunner(serviceClient, &staticKubeClientBuilder{privilegedClient: serviceClient}, &strings.Builder{}, "yes\n")
	var resolvedClusterID string
	runner.resolveServiceCluster = func(_ context.Context, resolvedID string) (ServiceClusterInfo, error) {
		resolvedClusterID = resolvedID
		return ServiceClusterInfo{ID: "service-cluster", Name: "service"}, nil
	}

	err := runner.moveVeleroResources(context.Background(), manifestWorkMoveFlags{clusterID: clusterID, replacementMC: "dne", reason: "OHSS-1234"})

	require.NoError(t, err, "expected Velero resources to move to a non-fleet namespace")
	assert.Equal(t, clusterID, resolvedClusterID, "expected canonical HCP ID for provision-shard resolution")
}

func TestSanitizeManifestWorkRemovesControllerMetadata(t *testing.T) {
	source := manifestWork("source-mc", "work")
	source.UID = types.UID("uid")
	source.ResourceVersion = "1"
	source.Generation = 2
	source.CreationTimestamp = metav1.NewTime(time.Now())
	source.Finalizers = []string{"work-finalizer"}
	source.OwnerReferences = []metav1.OwnerReference{{Name: "owner"}}
	source.Status.Conditions = []metav1.Condition{{Type: "Applied", Status: metav1.ConditionTrue}}

	destination := sanitizeManifestWork(source, "replacement-mc")

	assert.Empty(t, destination.UID, "expected destination UID to be removed")
	assert.Empty(t, destination.ResourceVersion, "expected destination resource version to be removed")
	assert.Zero(t, destination.Generation, "expected destination generation to be reset")
	assert.True(t, destination.CreationTimestamp.IsZero(), "expected destination creation timestamp to be removed")
	assert.Empty(t, destination.Finalizers, "expected destination finalizers to be removed")
	assert.Empty(t, destination.OwnerReferences, "expected destination owner references to be removed")
	assert.Empty(t, destination.Status.Conditions, "expected destination status to be removed")
}

func TestManifestWorksMatchComparesSecretPayloads(t *testing.T) {
	expected := drSecretWork("replacement-mc", "cluster-01-dr", "cluster")
	expected.Spec.Workload.Manifests[0] = manifestWithData("v1", "Secret", veleroNamespace, "cluster-dr-creds", "token", "expected")
	actual := expected.DeepCopy()
	assert.True(t, manifestWorksMatch(expected, actual), "expected identical Secret ManifestWorks to match")
	actual.Spec.Workload.Manifests[0] = manifestWithData("v1", "Secret", veleroNamespace, "cluster-dr-creds", "token", "different")

	assert.False(t, manifestWorksMatch(expected, actual), "expected a changed Secret payload to differ")
}

func TestManifestWorkMoveCommandsHaveRequiredFlagsInExamples(t *testing.T) {
	cmd := newCmdManifestWork()
	for _, path := range [][]string{{"move", "velero-resources"}, {"move", "cluster-resources"}} {
		leaf, _, err := cmd.Find(path)
		require.NoError(t, err, "expected ManifestWork command path %v", path)
		for _, name := range []string{"cluster-id", "replacement-mc", "reason"} {
			assert.Equal(t, "true", leaf.Flags().Lookup(name).Annotations[cobra.BashCompOneRequiredFlag][0], "expected %s to be required", name)
			assert.Contains(t, leaf.Example, "--"+name, "expected example to include required %s flag", name)
		}
	}
}

func TestManifestWorkCompareCommandHasRequiredFlagsInExample(t *testing.T) {
	cmd := newCmdManifestWorkCompare()
	for _, name := range []string{"cluster-id", "replacement-mc", "reason"} {
		assert.Equal(t, "true", cmd.Flags().Lookup(name).Annotations[cobra.BashCompOneRequiredFlag][0], "expected %s to be required", name)
		assert.Contains(t, cmd.Example, "--"+name, "expected example to include required %s flag", name)
	}
}

func testManifestWorkRunner(serviceClient KubeClient, builder KubeClientBuilder, out *strings.Builder, input string) *manifestWorkRunner {
	return &manifestWorkRunner{
		resolver:              &staticClusterResolver{clusterInfo: ClusterInfo{HCPClusterID: "cluster", MgmtClusterName: "source-mc"}},
		builder:               builder,
		resolveServiceCluster: staticServiceClusterResolver,
		in:                    strings.NewReader(input),
		out:                   out,
	}
}

func manifestWork(namespace, name string) *workv1.ManifestWork {
	return &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}, Spec: workv1.ManifestWorkSpec{Workload: workv1.ManifestsTemplate{}}}
}

func drSecretWork(namespace, name, clusterID string) *workv1.ManifestWork {
	return workWithManifests(namespace, name, clusterID, map[string]string{drSecretLabel: "true"}, manifest("v1", "Secret", veleroNamespace, clusterID+"-dr-creds"))
}

func drSchedulesWork(namespace, name, clusterID string) *workv1.ManifestWork {
	return workWithManifests(namespace, name, clusterID, nil, manifest(veleroAPIVersion, "Schedule", veleroNamespace, clusterID+"-daily"))
}

func namespaceWork(namespace, clusterID string) *workv1.ManifestWork {
	return workWithManifests(namespace, clusterID+namespaceManifestWorkTail, clusterID, map[string]string{containsNamespacesLabel: "true"})
}

func hostedClusterWork(namespace, clusterID string) *workv1.ManifestWork {
	return workWithManifests(namespace, clusterID, clusterID, map[string]string{hostedClusterLabel: "true"}, manifest(hostedClusterAPIVersion, hostedClusterKind, "cluster-ns", "cluster"))
}

func nodePoolWork(namespace, name, clusterID, label string) *workv1.ManifestWork {
	return workWithManifests(namespace, name, clusterID, map[string]string{label: "np"}, manifest(hostedClusterAPIVersion, nodePoolKind, "cluster-ns", name))
}

func workWithManifests(namespace, name, clusterID string, labels map[string]string, manifests ...workv1.Manifest) *workv1.ManifestWork {
	if labels == nil {
		labels = map[string]string{}
	}
	labels[clusterIDLabel] = clusterID
	labels[managementClusterLabel] = namespace
	return &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels}, Spec: workv1.ManifestWorkSpec{Workload: workv1.ManifestsTemplate{Manifests: manifests}}}
}

func manifest(apiVersion, kind, namespace, name string) workv1.Manifest {
	return manifestWithData(apiVersion, kind, namespace, name, "", "")
}

func manifestWithData(apiVersion, kind, namespace, name, key, value string) workv1.Manifest {
	object := map[string]any{"apiVersion": apiVersion, "kind": kind, "metadata": map[string]string{"namespace": namespace, "name": name}, "spec": map[string]any{}}
	if key != "" {
		object["data"] = map[string]string{key: value}
	}
	raw, _ := json.Marshal(object)
	return workv1.Manifest{RawExtension: runtime.RawExtension{Raw: raw}}
}

func assertPaused(t *testing.T, serviceClient KubeClient, namespace, name, kind string) {
	t.Helper()
	work := &workv1.ManifestWork{}
	require.NoError(t, serviceClient.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, work), "expected ManifestWork %s/%s", namespace, name)
	for i := range work.Spec.Workload.Manifests {
		object, err := manifestObject(&work.Spec.Workload.Manifests[i])
		require.NoError(t, err, "expected ManifestWork %s/%s payload to decode", namespace, name)
		if object.GetKind() == kind {
			paused, found, err := unstructured.NestedString(object.Object, "spec", "pausedUntil")
			require.NoError(t, err, "expected pausedUntil field lookup to succeed")
			require.True(t, found, "expected %s in %s/%s to include pausedUntil", kind, namespace, name)
			assert.Equal(t, "true", paused, "expected %s in %s/%s to remain paused", kind, namespace, name)
			return
		}
	}
	t.Fatalf("%s not found in %s/%s", kind, namespace, name)
}

func assertNotPaused(t *testing.T, serviceClient KubeClient, namespace, name, kind string) {
	t.Helper()
	work := &workv1.ManifestWork{}
	require.NoError(t, serviceClient.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, work), "expected ManifestWork %s/%s", namespace, name)
	for i := range work.Spec.Workload.Manifests {
		object, err := manifestObject(&work.Spec.Workload.Manifests[i])
		require.NoError(t, err, "expected ManifestWork %s/%s payload to decode", namespace, name)
		if object.GetKind() == kind {
			_, found, err := unstructured.NestedString(object.Object, "spec", "pausedUntil")
			require.NoError(t, err, "expected pausedUntil field lookup to succeed")
			assert.False(t, found, "expected %s in %s/%s to remain unpaused", kind, namespace, name)
			return
		}
	}
	t.Fatalf("%s not found in %s/%s", kind, namespace, name)
}

func assertManifestWorkNotFound(t *testing.T, serviceClient KubeClient, namespace, name string) {
	t.Helper()
	err := serviceClient.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, &workv1.ManifestWork{})
	assert.True(t, apierrors.IsNotFound(err), "expected ManifestWork %s/%s to be absent", namespace, name)
}

func newManifestWorkTestClient(t *testing.T, objects ...*workv1.ManifestWork) *testKubeClient {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, workv1.AddToScheme(scheme), "expected ManifestWork API to register with test scheme")
	items := make([]runtime.Object, 0, len(objects))
	for _, object := range objects {
		items = append(items, object)
	}
	return newTestClient(fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(items...).Build())
}

func staticServiceClusterResolver(_ context.Context, _ string) (ServiceClusterInfo, error) {
	return ServiceClusterInfo{ID: "service-cluster", Name: "service"}, nil
}
