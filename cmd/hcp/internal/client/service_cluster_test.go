package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestServiceClusterClientListManifestWorks(t *testing.T) {
	newClient := func(t *testing.T) *serviceClusterClient {
		t.Helper()
		scheme := runtime.NewScheme()
		require.NoError(t, workv1.AddToScheme(scheme))
		return &serviceClusterClient{client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Namespace: "management", Name: "cluster-b", Labels: map[string]string{"api.openshift.com/id": "cluster"}}},
			&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Namespace: "management", Name: "cluster-a", Labels: map[string]string{"api.openshift.com/id": "cluster"}}},
			&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Namespace: "management", Name: "other", Labels: map[string]string{"api.openshift.com/id": "other"}}},
		).Build()}
	}
	works, err := newClient(t).ListManifestWorks(context.Background(), "management", WithManifestWorkLabel("api.openshift.com/id", "cluster"))
	require.NoError(t, err)
	assert.Equal(t, []string{"cluster-a", "cluster-b"}, []string{works[0].Name, works[1].Name})
}

func TestServiceClusterClientUpdateManagedClusterHosting(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1.AddToScheme(scheme))
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{
		Name:            "managed",
		UID:             "managed-uid",
		ResourceVersion: "1",
		Annotations: map[string]string{
			"addon.open-cluster-management.io/hosting-cluster-name":  "source",
			"import.open-cluster-management.io/hosting-cluster-name": "source",
			"unrelated": "retained",
		},
	}}
	serviceClient := &serviceClusterClient{client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(managedCluster).Build()}

	require.NoError(t, serviceClient.UpdateManagedClusterHosting(context.Background(), "managed", "managed-uid", "1", "replacement", true))

	updated, err := serviceClient.GetManagedCluster(context.Background(), "managed")
	require.NoError(t, err)
	assert.Equal(t, "replacement", updated.Annotations["addon.open-cluster-management.io/hosting-cluster-name"])
	assert.Equal(t, "replacement", updated.Annotations["import.open-cluster-management.io/hosting-cluster-name"])
	assert.Equal(t, "retained", updated.Annotations["unrelated"])
	assert.Error(t, serviceClient.UpdateManagedClusterHosting(context.Background(), "managed", "managed-uid", "1", "other", true))
}

func TestServiceClusterClientUpdateManagedClusterHostingWithoutAddonAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1.AddToScheme(scheme))
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{
		Name: "managed", UID: "managed-uid", ResourceVersion: "1",
		Annotations: map[string]string{
			"import.open-cluster-management.io/hosting-cluster-name": "source",
		},
	}}
	serviceClient := &serviceClusterClient{client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(managedCluster).Build()}

	require.NoError(t, serviceClient.UpdateManagedClusterHosting(context.Background(), "managed", "managed-uid", "1", "replacement", false))

	updated, err := serviceClient.GetManagedCluster(context.Background(), "managed")
	require.NoError(t, err)
	assert.Equal(t, "replacement", updated.Annotations["import.open-cluster-management.io/hosting-cluster-name"])
	assert.NotContains(t, updated.Annotations, "addon.open-cluster-management.io/hosting-cluster-name")
}

func TestServiceClusterClientUpdateManagedClusterAddonHosting(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, addonv1alpha1.AddToScheme(scheme))
	addon := &addonv1alpha1.ManagedClusterAddOn{ObjectMeta: metav1.ObjectMeta{
		Namespace: "managed", Name: "work-manager", UID: "addon-uid",
		Annotations: map[string]string{"addon.open-cluster-management.io/hosting-cluster-name": "source", "unrelated": "retained"},
	}}
	serviceClient := &serviceClusterClient{client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(addon).Build()}

	addons, err := serviceClient.ListManagedClusterAddons(context.Background(), "managed")
	require.NoError(t, err)
	require.Len(t, addons, 1)
	require.NoError(t, serviceClient.UpdateManagedClusterAddonHosting(context.Background(), "managed", "work-manager", string(addons[0].UID), "source", "replacement"))

	updated, err := serviceClient.ListManagedClusterAddons(context.Background(), "managed")
	require.NoError(t, err)
	assert.Equal(t, "replacement", updated[0].Annotations["addon.open-cluster-management.io/hosting-cluster-name"])
	assert.Equal(t, "retained", updated[0].Annotations["unrelated"])
}

func TestServiceClusterClientOrphanDeleteManifestWork(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, workv1.AddToScheme(scheme))
	work := &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Namespace: "old-management", Name: "addon-work", UID: "work-uid"}}
	serviceClient := &serviceClusterClient{client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(work).Build()}

	require.NoError(t, serviceClient.OrphanDeleteManifestWork(context.Background(), work.Namespace, work.Name, string(work.UID)))
	_, err := serviceClient.GetManifestWork(context.Background(), work.Namespace, work.Name)
	assert.Error(t, err)
}
