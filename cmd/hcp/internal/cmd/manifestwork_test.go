package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestManifestWorkCompareComparesManagementClusters(t *testing.T) {
	sourceOnly := workWithManifests("source-mc", "a-work", "cluster", nil)
	sourceOnly.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue}}
	replacementOnly := workWithManifests("replacement-mc", "z-work", "cluster", nil)
	replacementOnly.Status.Conditions = []metav1.Condition{{Type: workv1.WorkAvailable, Status: metav1.ConditionTrue}}
	unrelated := workWithManifests("source-mc", "other-work", "other-cluster", nil)
	serviceClient := newManifestWorkTestClient(t, sourceOnly, replacementOnly, unrelated)
	ocmClient := &testManifestWorkOCMClient{cluster: ClusterInfo{HCPClusterID: "cluster", MgmtClusterName: "source-mc"}, serviceCluster: ServiceClusterInfo{ID: "service-cluster", Name: "service"}, client: serviceClient}
	output := &strings.Builder{}
	runner := testManifestWorkRunner(ocmClient, output, true)

	err := runner.compare(context.Background(), ManifestWorkRequest{ClusterID: "hcp", ReplacementMC: "replacement-mc", Reason: "OHSS-1234"})

	require.NoError(t, err, "expected ManifestWork comparison to succeed")
	require.Len(t, ocmClient.calls, 1, "expected one service-cluster connection")
	assert.Equal(t, "service-cluster", ocmClient.calls[0].clusterID, "expected service cluster ID")
	assert.Equal(t, "OHSS-1234", ocmClient.calls[0].reason, "expected elevation reason to be forwarded")
	assert.Equal(t, []string{"cluster"}, ocmClient.resolved, "expected canonical HCP ID for service-cluster resolution")
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
	ocmClient := &testManifestWorkOCMClient{cluster: ClusterInfo{HCPClusterID: clusterID, MgmtClusterName: "source-mc"}, serviceCluster: ServiceClusterInfo{ID: "service-cluster", Name: "service"}, client: serviceClient}
	runner := testManifestWorkRunner(ocmClient, &strings.Builder{}, true)

	err := runner.moveVeleroResources(context.Background(), ManifestWorkRequest{ClusterID: clusterID, ReplacementMC: "replacement-mc", Reason: "OHSS-1234"})

	require.NoError(t, err, "expected Velero resources to move")
	require.Len(t, ocmClient.calls, 1, "expected one service-cluster connection")
	assert.Equal(t, "service-cluster", ocmClient.calls[0].clusterID, "expected service cluster ID")
	assert.Equal(t, "OHSS-1234", ocmClient.calls[0].reason, "expected elevation reason to be forwarded")
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
	runner := testManifestWorkRunnerForClient(serviceClient, &strings.Builder{}, true)

	err := runner.moveClusterResources(context.Background(), ManifestWorkRequest{ClusterID: "cluster", ReplacementMC: "replacement-mc", Reason: "OHSS-1234"})

	require.ErrorContains(t, err, "Velero resource ManifestWork", "expected missing Velero prerequisite to stop the cluster move")
}

func TestManifestWorkMoveClusterResourcesLeavesDestinationHostedClusterUnpaused(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	sourceHostedCluster := hostedClusterWork("source-mc", clusterID)
	require.NoError(t, pauseManifestWorkPayload(sourceHostedCluster), "failed to prepare paused source HostedCluster work")
	works := []*workv1.ManifestWork{
		drSecretWork("replacement-mc", secretName, clusterID),
		drSchedulesWork("replacement-mc", schedulesName, clusterID),
		namespaceWork("source-mc", clusterID),
		sourceHostedCluster,
		nodePoolWork("source-mc", clusterID+"-np-a", clusterID, nodePoolOCMLabel),
		nodePoolWork("source-mc", clusterID+"-np-b", clusterID, nodePoolLabel),
		nodePoolWork("source-mc", "other-np", "other", nodePoolOCMLabel),
	}
	serviceClient := newManifestWorkTestClient(t, works...)
	runner := testManifestWorkRunnerForClient(serviceClient, &strings.Builder{}, true)

	err := runner.moveClusterResources(context.Background(), ManifestWorkRequest{ClusterID: clusterID, ReplacementMC: "replacement-mc", Reason: "OHSS-1234"})

	require.NoError(t, err, "expected cluster resources to move")
	for _, name := range []string{clusterID + namespaceManifestWorkTail, clusterID, clusterID + "-np-a", clusterID + "-np-b"} {
		destination := &workv1.ManifestWork{}
		require.NoError(t, serviceClient.Get(context.Background(), types.NamespacedName{Namespace: "replacement-mc", Name: name}, destination), "expected destination ManifestWork %s", name)
		assert.Equal(t, "replacement-mc", destination.Labels[managementClusterLabel], "expected destination management-cluster label for %s", name)
	}
	assertPaused(t, serviceClient, "source-mc", clusterID, hostedClusterKind)
	assertPaused(t, serviceClient, "source-mc", clusterID+"-np-a", nodePoolKind)
	assertNotPaused(t, serviceClient, "replacement-mc", clusterID, hostedClusterKind)
	assertPaused(t, serviceClient, "replacement-mc", clusterID+"-np-b", nodePoolKind)
	assertManifestWorkNotFound(t, serviceClient, "replacement-mc", "other-np")
}

func TestManifestWorkMoveClusterResourcesPausesDestinationHostedClusterWhenRequested(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	serviceClient := newManifestWorkTestClient(t,
		drSecretWork("replacement-mc", secretName, clusterID),
		drSchedulesWork("replacement-mc", schedulesName, clusterID),
		namespaceWork("source-mc", clusterID),
		hostedClusterWork("source-mc", clusterID),
	)
	runner := testManifestWorkRunnerForClient(serviceClient, &strings.Builder{}, true)

	err := runner.moveClusterResources(context.Background(), ManifestWorkRequest{ClusterID: clusterID, ReplacementMC: "replacement-mc", Reason: "OHSS-1234", Paused: true})

	require.NoError(t, err, "expected cluster resources to move")
	assertPaused(t, serviceClient, "replacement-mc", clusterID, hostedClusterKind)
}

func TestManifestWorkMoveClusterResourcesRejectsSourceChangedAfterPlanning(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	serviceClient := newManifestWorkTestClient(t,
		drSecretWork("replacement-mc", secretName, clusterID),
		drSchedulesWork("replacement-mc", schedulesName, clusterID),
		namespaceWork("source-mc", clusterID),
		hostedClusterWork("source-mc", clusterID),
	)
	runner := testManifestWorkRunnerForClient(serviceClient, &strings.Builder{}, true)
	plan, err := runner.manager.PrepareClusterManifestWorkMove(context.Background(), ManifestWorkRequest{ClusterID: clusterID, ReplacementMC: "replacement-mc", Reason: "OHSS-1234"})
	require.NoError(t, err, "expected cluster move plan")

	source, err := serviceClient.GetManifestWork(context.Background(), "source-mc", clusterID)
	require.NoError(t, err, "expected source HostedCluster work")
	source.Annotations = map[string]string{"changed": "after-planning"}
	require.NoError(t, serviceClient.Update(context.Background(), source), "expected source change")

	_, err = runner.manager.MoveManifestWorks(context.Background(), plan)

	require.ErrorContains(t, err, "changed since planning")
	assertNotPaused(t, serviceClient, "source-mc", clusterID, hostedClusterKind)
	assertManifestWorkNotFound(t, serviceClient, "replacement-mc", clusterID)
}

func TestManifestWorkMoveClusterResourcesWaitsForSourcePause(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	serviceClient := newManifestWorkTestClient(t,
		drSecretWork("replacement-mc", secretName, clusterID),
		drSchedulesWork("replacement-mc", schedulesName, clusterID),
		namespaceWork("source-mc", clusterID),
		hostedClusterWork("source-mc", clusterID),
	)
	serviceClient.acknowledgePause = false
	runner := testManifestWorkRunnerForClient(serviceClient, &strings.Builder{}, true)
	runner.manager = NewDefaultManifestWorkManager(runner.manager.ocmClient,
		WithManifestWorkPauseTimeout(20*time.Millisecond),
		WithManifestWorkPauseAcknowledgmentPeriod(time.Millisecond),
	)
	plan, err := runner.manager.PrepareClusterManifestWorkMove(context.Background(), ManifestWorkRequest{ClusterID: clusterID, ReplacementMC: "replacement-mc", Reason: "OHSS-1234"})
	require.NoError(t, err, "expected cluster move plan")

	_, err = runner.manager.MoveManifestWorks(context.Background(), plan)

	require.ErrorContains(t, err, "waiting for source HostedCluster ManifestWork")
	assertManifestWorkNotFound(t, serviceClient, "replacement-mc", clusterID)
}

func TestValidateManifestWorkPauseAcknowledgmentRejectsNewGeneration(t *testing.T) {
	work := hostedClusterWork("source-mc", "cluster")
	work.Generation = 2
	work.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue, ObservedGeneration: 2}}
	serviceClient := newManifestWorkTestClient(t, work)

	require.NoError(t, validateManifestWorkPauseAcknowledgment(context.Background(), serviceClient, "source-mc", "cluster", 2))

	current, err := serviceClient.GetManifestWork(context.Background(), "source-mc", "cluster")
	require.NoError(t, err)
	current.Generation = 3
	require.NoError(t, serviceClient.Update(context.Background(), current))

	err = validateManifestWorkPauseAcknowledgment(context.Background(), serviceClient, "source-mc", "cluster", 2)
	require.ErrorContains(t, err, "changed after pause acknowledgement")
}

func TestManifestWorkMoveHonorsCanceledTargetLookup(t *testing.T) {
	ocmClient := &testManifestWorkOCMClient{cluster: ClusterInfo{HCPClusterID: "cluster", MgmtClusterName: "source-mc"}}
	manager := NewDefaultManifestWorkManager(ocmClient)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := manager.PrepareVeleroManifestWorkMove(ctx, ManifestWorkRequest{ClusterID: "cluster", ReplacementMC: "replacement-mc", Reason: "OHSS-1234"})

	require.ErrorIs(t, err, context.Canceled)
	assert.Same(t, ctx, ocmClient.clusterContext, "expected target lookup to receive the caller context")
}

func TestDefaultManifestWorkManagerOptions(t *testing.T) {
	ocmClient := &testManifestWorkOCMClient{}
	manager := NewDefaultManifestWorkManager(ocmClient)
	assert.Equal(t, 5*time.Minute, manager.cfg.PauseTimeout)
	assert.Equal(t, time.Second, manager.cfg.PauseAcknowledgmentPeriod)

	manager = NewDefaultManifestWorkManager(ocmClient,
		WithManifestWorkPauseTimeout(time.Minute),
		WithManifestWorkPauseAcknowledgmentPeriod(5*time.Second),
	)
	assert.Equal(t, time.Minute, manager.cfg.PauseTimeout)
	assert.Equal(t, 5*time.Second, manager.cfg.PauseAcknowledgmentPeriod)
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
	runner := testManifestWorkRunnerForClient(serviceClient, &strings.Builder{}, false)

	err := runner.moveClusterResources(context.Background(), ManifestWorkRequest{ClusterID: clusterID, ReplacementMC: "replacement-mc", Reason: "OHSS-1234"})

	require.NoError(t, err, "expected declined cluster move to return without error")
	assertNotPaused(t, serviceClient, "source-mc", clusterID, hostedClusterKind)
	assertManifestWorkNotFound(t, serviceClient, "replacement-mc", clusterID)
}

func TestManifestWorkMoveClusterResourcesRejectsConflictBeforePausingSource(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	conflictingHostedCluster := hostedClusterWork("replacement-mc", clusterID)
	require.NoError(t, pauseManifestWorkPayload(conflictingHostedCluster), "failed to prepare conflicting destination HostedCluster work")
	serviceClient := newManifestWorkTestClient(t,
		drSecretWork("replacement-mc", secretName, clusterID),
		drSchedulesWork("replacement-mc", schedulesName, clusterID),
		namespaceWork("source-mc", clusterID),
		hostedClusterWork("source-mc", clusterID),
		conflictingHostedCluster,
	)
	runner := testManifestWorkRunnerForClient(serviceClient, &strings.Builder{}, true)

	err := runner.moveClusterResources(context.Background(), ManifestWorkRequest{ClusterID: clusterID, ReplacementMC: "replacement-mc", Reason: "OHSS-1234"})

	require.ErrorContains(t, err, "different content", "expected conflicting destination to stop the move")
	assertNotPaused(t, serviceClient, "source-mc", clusterID, hostedClusterKind)
}

func TestManifestWorkMoveClusterResourcesAcceptsPausedDestinationWhenRequested(t *testing.T) {
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
	runner := testManifestWorkRunnerForClient(serviceClient, &strings.Builder{}, true)

	err := runner.moveClusterResources(context.Background(), ManifestWorkRequest{ClusterID: clusterID, ReplacementMC: "replacement-mc", Reason: "OHSS-1234", Paused: true})

	require.NoError(t, err, "expected existing paused destination to be accepted")
	assertPaused(t, serviceClient, "source-mc", clusterID, hostedClusterKind)
}

func TestManifestWorkMoveRejectsConflictingDestination(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	conflict := drSecretWork("replacement-mc", secretName, clusterID)
	conflict.Spec.Workload.Manifests = nil
	serviceClient := newManifestWorkTestClient(t, drSecretWork("source-mc", secretName, clusterID), drSchedulesWork("source-mc", schedulesName, clusterID), conflict)
	runner := testManifestWorkRunnerForClient(serviceClient, &strings.Builder{}, true)

	err := runner.moveVeleroResources(context.Background(), ManifestWorkRequest{ClusterID: clusterID, ReplacementMC: "replacement-mc", Reason: "OHSS-1234"})

	require.ErrorContains(t, err, "different content", "expected conflicting destination to be rejected")
}

func TestManifestWorkMoveUsesHCPProvisionShardServiceCluster(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	serviceClient := newManifestWorkTestClient(t, drSecretWork("source-mc", secretName, clusterID), drSchedulesWork("source-mc", schedulesName, clusterID))
	ocmClient := &testManifestWorkOCMClient{
		cluster:        ClusterInfo{HCPClusterID: clusterID, MgmtClusterName: "source-mc"},
		serviceCluster: ServiceClusterInfo{ID: "service-cluster", Name: "service"},
		client:         serviceClient,
	}
	runner := testManifestWorkRunner(ocmClient, &strings.Builder{}, true)

	err := runner.moveVeleroResources(context.Background(), ManifestWorkRequest{ClusterID: clusterID, ReplacementMC: "dne", Reason: "OHSS-1234"})

	require.NoError(t, err, "expected Velero resources to move to the resolved namespace")
	assert.Equal(t, []string{clusterID}, ocmClient.resolved, "expected canonical HCP ID for provision-shard resolution")
}

func TestManifestWorkMoveResolvesTargetManagementClusterID(t *testing.T) {
	clusterID := "cluster"
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	serviceClient := newManifestWorkTestClient(t, drSecretWork("source-mc", secretName, clusterID), drSchedulesWork("source-mc", schedulesName, clusterID))
	target, err := cmv1.NewCluster().ID("target-id").Name("target-mc").Build()
	require.NoError(t, err, "expected target management cluster")
	ocmClient := &testManifestWorkOCMClient{
		cluster:            ClusterInfo{HCPClusterID: clusterID, MgmtClusterName: "source-mc"},
		serviceCluster:     ServiceClusterInfo{ID: "service-cluster", Name: "service"},
		client:             serviceClient,
		managementClusters: map[string]*cmv1.Cluster{"target-id": target},
	}
	runner := testManifestWorkRunner(ocmClient, &strings.Builder{}, true)

	err = runner.moveVeleroResources(context.Background(), ManifestWorkRequest{ClusterID: clusterID, ReplacementMC: "target-id", Reason: "OHSS-1234"})

	require.NoError(t, err, "expected target MC ID to resolve")
	destination := &workv1.ManifestWork{}
	require.NoError(t, serviceClient.Get(context.Background(), types.NamespacedName{Namespace: "target-mc", Name: secretName}, destination), "expected destination in resolved target namespace")
	assert.Equal(t, "target-mc", destination.Labels[managementClusterLabel], "expected canonical target management-cluster label")
}

func TestManifestWorkMoveRejectsTargetResolvingToSource(t *testing.T) {
	target, err := cmv1.NewCluster().ID("source-id").Name("source-mc").Build()
	require.NoError(t, err, "expected source management cluster")
	ocmClient := &testManifestWorkOCMClient{
		cluster:            ClusterInfo{HCPClusterID: "cluster", MgmtClusterName: "source-mc"},
		managementClusters: map[string]*cmv1.Cluster{"source-id": target},
	}
	runner := testManifestWorkRunner(ocmClient, &strings.Builder{}, true)

	err = runner.moveVeleroResources(context.Background(), ManifestWorkRequest{ClusterID: "cluster", ReplacementMC: "source-id", Reason: "OHSS-1234"})

	require.ErrorContains(t, err, "source and replacement management clusters must differ")
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

type serviceClusterCall struct {
	clusterID string
	reason    string
}

// manifestWorkRunner preserves concise test setup while production uses the
// IO-free DefaultManifestWorkManager directly.
type manifestWorkRunner struct {
	manager *DefaultManifestWorkManager
	prompt  *recordingPrompt
	out     *strings.Builder
}

func (r *manifestWorkRunner) compare(ctx context.Context, request ManifestWorkRequest) error {
	comparisons, err := r.manager.CompareManifestWorks(ctx, request)
	if err != nil {
		return err
	}
	table := printer.NewTablePrinter(r.out, 0, 4, 2, ' ')
	table.AddRow([]string{"NAME", "SOURCE", "SOURCE APPLIED", "SOURCE AVAILABLE", "REPLACEMENT", "REPLACEMENT APPLIED", "REPLACEMENT AVAILABLE"})
	for _, comparison := range comparisons {
		table.AddRow([]string{comparison.Name, presenceStatus(comparison.Source), conditionStatusForWork(comparison.Source, workv1.WorkApplied), conditionStatusForWork(comparison.Source, workv1.WorkAvailable), presenceStatus(comparison.Replacement), conditionStatusForWork(comparison.Replacement, workv1.WorkApplied), conditionStatusForWork(comparison.Replacement, workv1.WorkAvailable)})
	}
	return table.Flush()
}

func (r *manifestWorkRunner) moveVeleroResources(ctx context.Context, request ManifestWorkRequest) error {
	plan, err := r.manager.PrepareVeleroManifestWorkMove(ctx, request)
	if err != nil || len(plan.ManifestWorks) == 0 || !r.prompt.Confirm("") {
		return err
	}
	_, err = r.manager.MoveManifestWorks(ctx, plan)
	return err
}

func (r *manifestWorkRunner) moveClusterResources(ctx context.Context, request ManifestWorkRequest) error {
	plan, err := r.manager.PrepareClusterManifestWorkMove(ctx, request)
	if err != nil || len(plan.ManifestWorks) == 0 || !r.prompt.Confirm("") {
		return err
	}
	_, err = r.manager.MoveManifestWorks(ctx, plan)
	return err
}

func testManifestWorkRunner(ocmClient manifestWorkOCMClient, out *strings.Builder, confirmed bool) *manifestWorkRunner {
	return &manifestWorkRunner{
		manager: NewDefaultManifestWorkManager(ocmClient),
		prompt:  &recordingPrompt{confirmed: confirmed},
		out:     out,
	}
}

func testManifestWorkRunnerForClient(serviceClient ServiceClusterClient, out *strings.Builder, confirmed bool) *manifestWorkRunner {
	return testManifestWorkRunner(&testManifestWorkOCMClient{
		cluster:        ClusterInfo{HCPClusterID: "cluster", MgmtClusterName: "source-mc"},
		serviceCluster: ServiceClusterInfo{ID: "service-cluster", Name: "service"},
		client:         serviceClient,
	}, out, confirmed)
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

func assertPaused(t *testing.T, serviceClient *manifestWorkTestClient, namespace, name, kind string) {
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

func assertNotPaused(t *testing.T, serviceClient *manifestWorkTestClient, namespace, name, kind string) {
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

func assertManifestWorkNotFound(t *testing.T, serviceClient *manifestWorkTestClient, namespace, name string) {
	t.Helper()
	err := serviceClient.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, &workv1.ManifestWork{})
	assert.True(t, apierrors.IsNotFound(err), "expected ManifestWork %s/%s to be absent", namespace, name)
}

type manifestWorkTestClient struct {
	client.Client
	acknowledgePause bool
}

func newManifestWorkTestClient(t *testing.T, objects ...*workv1.ManifestWork) *manifestWorkTestClient {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, workv1.AddToScheme(scheme), "expected ManifestWork API to register with test scheme")
	items := make([]runtime.Object, 0, len(objects))
	for _, object := range objects {
		items = append(items, object)
	}
	runtimeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(items...).Build()
	return &manifestWorkTestClient{Client: runtimeClient, acknowledgePause: true}
}

func (c *manifestWorkTestClient) GetManifestWork(ctx context.Context, namespace, name string) (*workv1.ManifestWork, error) {
	work := &workv1.ManifestWork{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, work); err != nil {
		return nil, err
	}
	return work, nil
}

func (c *manifestWorkTestClient) ListManifestWorks(ctx context.Context, namespace string, options ...internalclient.ManifestWorkListOption) ([]workv1.ManifestWork, error) {
	list := &workv1.ManifestWorkList{}
	listOptions := []client.ListOption{client.InNamespace(namespace)}
	if labels := internalclient.ManifestWorkLabels(options...); len(labels) > 0 {
		listOptions = append(listOptions, client.MatchingLabels(labels))
	}
	if err := c.List(ctx, list, listOptions...); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *manifestWorkTestClient) CreateManifestWork(ctx context.Context, work *workv1.ManifestWork) error {
	return c.Create(ctx, work)
}

func (c *manifestWorkTestClient) UpdateManifestWork(ctx context.Context, work *workv1.ManifestWork) error {
	if err := c.Update(ctx, work); err != nil || !c.acknowledgePause {
		return err
	}
	updated, err := c.GetManifestWork(ctx, work.Namespace, work.Name)
	if err != nil {
		return err
	}
	updated.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue, ObservedGeneration: updated.Generation}}
	return c.Update(ctx, updated)
}
