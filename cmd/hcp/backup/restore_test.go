package backup

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewVeleroRestore(t *testing.T) {
	restore := newVeleroRestore(testVeleroResources(), "cluster-id", "daily-backup", 2)

	assert.Equal(t, "velero.io/v1", restore.GetAPIVersion())
	assert.Equal(t, "Restore", restore.GetKind())
	assert.Equal(t, veleroNamespace, restore.GetNamespace())
	assert.Equal(t, "daily-backup", restore.GetAnnotations()[restoreBackupAnnotation])
	assert.Equal(t, "cluster-id", restore.GetAnnotations()[restoreClusterAnnotation])
	assert.Equal(t, "2", restore.GetAnnotations()[restoreAttemptAnnotation])
	assert.Equal(t, restoreManagedValue, restore.GetLabels()[restoreManagedLabel])
	backupName, found, err := unstructured.NestedString(restore.Object, "spec", "backupName")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "daily-backup", backupName)
	assert.Equal(t, restoreName("cluster-id", "daily-backup", 2), restore.GetName())
}

func TestDiscoverVeleroResourcesUsesPreferredVersion(t *testing.T) {
	kubeClient := &testKubeClient{
		serverGroupsFn: func() (*metav1.APIGroupList, error) {
			return &metav1.APIGroupList{Groups: []metav1.APIGroup{{
				Name:             veleroAPIGroup,
				PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "velero.io/v2", Version: "v2"},
			}}}, nil
		},
		serverResourcesForGroupVersionFn: func(groupVersion string) (*metav1.APIResourceList, error) {
			assert.Equal(t, "velero.io/v2", groupVersion)
			return &metav1.APIResourceList{APIResources: []metav1.APIResource{
				{Name: "backups", Kind: "Backup", Namespaced: true},
				{Name: "restores", Kind: "Restore", Namespaced: true},
				{Name: "backupstoragelocations", Kind: "BackupStorageLocation", Namespaced: true},
			}}, nil
		},
	}

	resources, err := discoverVeleroResources(kubeClient)

	require.NoError(t, err)
	assert.Equal(t, "v2", resources.groupVersion.Version)
	assert.Equal(t, "Restore", resources.restoreKind)
}

func TestDiscoverVeleroResourcesRejectsUnavailableResources(t *testing.T) {
	kubeClient := &testKubeClient{
		serverGroupsFn: func() (*metav1.APIGroupList, error) {
			return &metav1.APIGroupList{Groups: []metav1.APIGroup{{
				Name:             veleroAPIGroup,
				PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "velero.io/v1", Version: "v1"},
			}}}, nil
		},
		serverResourcesForGroupVersionFn: func(string) (*metav1.APIResourceList, error) {
			return &metav1.APIResourceList{APIResources: []metav1.APIResource{{Name: "backups", Kind: "Backup", Namespaced: true}}}, nil
		},
	}

	_, err := discoverVeleroResources(kubeClient)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required resources")
}

func TestDiscoverVeleroResourcesReportsDiscoveryFailure(t *testing.T) {
	kubeClient := &testKubeClient{serverGroupsFn: func() (*metav1.APIGroupList, error) {
		return nil, errors.New("discovery unavailable")
	}}

	_, err := discoverVeleroResources(kubeClient)

	require.Error(t, err)
	assert.ErrorContains(t, err, "discovery unavailable")
}

func TestRestoreRunnerRunUsesBoundManagementCluster(t *testing.T) {
	backup := completedVeleroObject("Backup", "backup")
	location := availableBackupStorageLocation("default")
	restore := completedVeleroObject("Restore", restoreName("hcp", "backup", 1))
	restore.SetLabels(map[string]string{restoreManagedLabel: restoreManagedValue})
	restore.SetAnnotations(map[string]string{restoreBackupAnnotation: "backup", restoreClusterAnnotation: "hcp", restoreAttemptAnnotation: "1"})
	kubeClient := newTestClient(fake.NewClientBuilder().WithRuntimeObjects(backup, location, restore).Build())
	builder := &staticKubeClientBuilder{privilegedClient: kubeClient}
	output := &bytes.Buffer{}
	runner := &restoreRunner{
		resolver:     &staticClusterResolver{clusterInfo: ClusterInfo{HCPClusterID: "hcp", MgmtClusterID: "bound-mc"}},
		builder:      builder,
		out:          output,
		pollInterval: time.Millisecond,
	}

	err := runner.run(context.Background(), restoreFlags{clusterID: "hcp", backupName: "backup", reason: "DR", timeout: time.Second})

	require.NoError(t, err)
	require.Len(t, builder.calls, 1)
	assert.Equal(t, "bound-mc", builder.calls[0].clusterID)
	assert.True(t, builder.calls[0].elevated)
	assert.Equal(t, "DR", builder.calls[0].elevationReason)
	assert.Contains(t, output.String(), "completed")
}

func TestRestoreRunnerRunRejectsInvalidTimeoutBeforeResolvingCluster(t *testing.T) {
	builder := &staticKubeClientBuilder{}
	runner := &restoreRunner{
		resolver: &staticClusterResolver{err: errors.New("should not resolve cluster")},
		builder:  builder,
		out:      &bytes.Buffer{},
	}

	err := runner.run(context.Background(), restoreFlags{timeout: 0})

	require.Error(t, err)
	assert.ErrorContains(t, err, "timeout must be greater than zero")
	assert.Empty(t, builder.calls)
}

func TestRestoreRunnerRunUsesExplicitTargetManagementCluster(t *testing.T) {
	location := availableBackupStorageLocation("default")
	restore := completedVeleroObject("Restore", restoreName("hcp", "backup", 1))
	restore.SetLabels(map[string]string{restoreManagedLabel: restoreManagedValue})
	restore.SetAnnotations(map[string]string{restoreBackupAnnotation: "backup", restoreClusterAnnotation: "hcp", restoreAttemptAnnotation: "1"})
	kubeClient := newTestClient(fake.NewClientBuilder().WithRuntimeObjects(location, restore).Build())
	builder := &staticKubeClientBuilder{privilegedClient: kubeClient}
	target, err := cmv1.NewCluster().
		ID("target-id").
		Name("target").
		Region(cmv1.NewCloudRegion().ID("us-east-1")).
		Build()
	require.NoError(t, err)
	runner := &restoreRunner{
		resolver:     &staticClusterResolver{clusterInfo: ClusterInfo{HCPClusterID: "hcp", HCPClusterRegion: "us-east-1", MgmtClusterID: "bound-mc"}},
		getCluster:   func(string) (*cmv1.Cluster, error) { return target, nil },
		builder:      builder,
		out:          &bytes.Buffer{},
		pollInterval: time.Millisecond,
	}

	err = runner.run(context.Background(), restoreFlags{clusterID: "hcp", targetMC: "target", backupName: "backup", reason: "DR", timeout: time.Second})

	require.NoError(t, err)
	require.Len(t, builder.calls, 1)
	assert.Equal(t, "target-id", builder.calls[0].clusterID)
}

func TestRestoreRunnerRunRejectsTargetInDifferentRegion(t *testing.T) {
	target, err := cmv1.NewCluster().
		ID("target-id").
		Name("target").
		Region(cmv1.NewCloudRegion().ID("us-west-2")).
		Build()
	require.NoError(t, err)
	builder := &staticKubeClientBuilder{}
	runner := &restoreRunner{
		resolver:   &staticClusterResolver{clusterInfo: ClusterInfo{HCPClusterID: "hcp", HCPClusterRegion: "us-east-1", MgmtClusterID: "bound-mc"}},
		getCluster: func(string) (*cmv1.Cluster, error) { return target, nil },
		builder:    builder,
		out:        &bytes.Buffer{},
	}

	err = runner.run(context.Background(), restoreFlags{clusterID: "hcp", targetMC: "target", backupName: "backup", reason: "DR", timeout: time.Second})

	require.Error(t, err)
	assert.ErrorContains(t, err, "region us-west-2, expected HCP region us-east-1")
	assert.Empty(t, builder.calls)
}

func TestRestoreRunnerRunRequiresCompletedBackupOnBoundManagementCluster(t *testing.T) {
	tests := []struct {
		name    string
		objects []client.Object
		wantErr string
	}{
		{
			name:    "missing backup",
			objects: []client.Object{availableBackupStorageLocation("default")},
			wantErr: "was not found",
		},
		{
			name: "incomplete backup",
			objects: []client.Object{
				availableBackupStorageLocation("default"),
				veleroObject(testVeleroResources(), "Backup", "backup"),
			},
			wantErr: "expected Completed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeClient := newTestClient(fake.NewClientBuilder().WithObjects(tt.objects...).Build())
			runner := &restoreRunner{
				resolver: &staticClusterResolver{clusterInfo: ClusterInfo{HCPClusterID: "hcp", MgmtClusterID: "bound-mc"}},
				builder:  &staticKubeClientBuilder{privilegedClient: kubeClient},
				out:      &bytes.Buffer{},
			}

			err := runner.run(context.Background(), restoreFlags{clusterID: "hcp", backupName: "backup", reason: "DR", timeout: time.Second})

			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestRestoreRunnerRunRequiresAvailableBackupStorageLocation(t *testing.T) {
	kubeClient := newTestClient(fake.NewClientBuilder().WithRuntimeObjects(completedVeleroObject("Backup", "backup")).Build())
	runner := &restoreRunner{
		resolver: &staticClusterResolver{clusterInfo: ClusterInfo{HCPClusterID: "hcp", MgmtClusterID: "mc"}},
		builder:  &staticKubeClientBuilder{privilegedClient: kubeClient},
		out:      &bytes.Buffer{},
	}

	err := runner.run(context.Background(), restoreFlags{clusterID: "hcp", backupName: "backup", reason: "DR", timeout: time.Second})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no BackupStorageLocations")
}

func TestRestoreRunnerGetOrCreateRestoreSkipsCompletedAndResumesActive(t *testing.T) {
	tests := []struct {
		name  string
		phase string
	}{
		{name: "completed", phase: "Completed"},
		{name: "active", phase: "InProgress"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := completedVeleroObject("Restore", restoreName("hcp", "backup", 1))
			restore.Object["status"] = map[string]interface{}{"phase": tt.phase}
			restore.SetLabels(map[string]string{restoreManagedLabel: restoreManagedValue})
			restore.SetAnnotations(map[string]string{restoreBackupAnnotation: "backup", restoreClusterAnnotation: "hcp", restoreAttemptAnnotation: "1"})
			kubeClient := newTestClient(fake.NewClientBuilder().WithRuntimeObjects(restore).Build())
			runner := &restoreRunner{}

			actual, created, err := runner.getOrCreateRestore(context.Background(), kubeClient, testVeleroResources(), "hcp", "backup")

			require.NoError(t, err)
			assert.False(t, created)
			assert.Equal(t, restore.GetName(), actual.GetName())
		})
	}
}

func TestRestoreRunnerGetOrCreateRestoreRetriesFailedAttempt(t *testing.T) {
	for _, phase := range []string{"Failed", "FailedValidation"} {
		t.Run(phase, func(t *testing.T) {
			failed := completedVeleroObject("Restore", restoreName("hcp", "backup", 1))
			failed.Object["status"] = map[string]interface{}{"phase": phase}
			failed.SetLabels(map[string]string{restoreManagedLabel: restoreManagedValue})
			failed.SetAnnotations(map[string]string{restoreBackupAnnotation: "backup", restoreClusterAnnotation: "hcp", restoreAttemptAnnotation: "1"})
			kubeClient := newTestClient(fake.NewClientBuilder().WithRuntimeObjects(failed).Build())

			restore, created, err := (&restoreRunner{}).getOrCreateRestore(context.Background(), kubeClient, testVeleroResources(), "hcp", "backup")

			require.NoError(t, err)
			assert.True(t, created)
			assert.Equal(t, restoreName("hcp", "backup", 2), restore.GetName())
		})
	}
}

func TestRestoreRunnerGetOrCreateRestoreIgnoresOtherCluster(t *testing.T) {
	otherClusterRestore := completedVeleroObject("Restore", restoreName("other-hcp", "backup", 1))
	otherClusterRestore.SetLabels(map[string]string{restoreManagedLabel: restoreManagedValue})
	otherClusterRestore.SetAnnotations(map[string]string{
		restoreBackupAnnotation:  "backup",
		restoreClusterAnnotation: "other-hcp",
		restoreAttemptAnnotation: "1",
	})
	kubeClient := newTestClient(fake.NewClientBuilder().WithRuntimeObjects(otherClusterRestore).Build())

	restore, created, err := (&restoreRunner{}).getOrCreateRestore(context.Background(), kubeClient, testVeleroResources(), "hcp", "backup")

	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, restoreName("hcp", "backup", 1), restore.GetName())
}

func TestRestoreRunnerWaitForRestoreReportsTerminalFailure(t *testing.T) {
	for _, phase := range []string{"FailedValidation", "PartiallyFailed"} {
		t.Run(phase, func(t *testing.T) {
			restore := completedVeleroObject("Restore", "restore")
			restore.Object["status"] = map[string]interface{}{"phase": phase, "failureReason": "pod volume restore failed"}
			kubeClient := newTestClient(fake.NewClientBuilder().WithRuntimeObjects(restore).Build())

			err := (&restoreRunner{pollInterval: time.Millisecond}).waitForRestore(context.Background(), kubeClient, testVeleroResources(), "restore", "backup")

			require.Error(t, err)
			assert.Contains(t, err.Error(), phase)
			assert.Contains(t, err.Error(), "pod volume restore failed")
		})
	}
}

func TestRestoreRunnerWaitForRestoreReportsCancellation(t *testing.T) {
	restore := completedVeleroObject("Restore", "restore")
	restore.Object["status"] = map[string]interface{}{"phase": "InProgress"}
	kubeClient := newTestClient(fake.NewClientBuilder().WithRuntimeObjects(restore).Build())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := (&restoreRunner{pollInterval: time.Millisecond}).waitForRestore(ctx, kubeClient, testVeleroResources(), "restore", "backup")

	require.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, err.Error(), "restore")
}

func TestRestoreCommandRequiresInputs(t *testing.T) {
	cmd := newCmdRestore()
	cmd.SetArgs(nil)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "required flag"))
}

func completedVeleroObject(kind, name string) *unstructured.Unstructured {
	obj := veleroObject(testVeleroResources(), kind, name)
	obj.Object["status"] = map[string]interface{}{"phase": "Completed"}
	return obj
}

func availableBackupStorageLocation(name string) *unstructured.Unstructured {
	obj := veleroObject(testVeleroResources(), "BackupStorageLocation", name)
	obj.Object["status"] = map[string]interface{}{"phase": "Available"}
	return obj
}

func testVeleroResources() veleroResources {
	return veleroResources{
		groupVersion: schema.GroupVersion{Group: veleroAPIGroup, Version: "v1"},
		backupKind:   "Backup",
		restoreKind:  "Restore",
		bslKind:      "BackupStorageLocation",
	}
}
