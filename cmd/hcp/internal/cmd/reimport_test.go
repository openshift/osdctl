package cmd

import (
	"context"
	"fmt"
	"testing"
	"time"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

type reimportTestClient struct {
	managedCluster *clusterv1.ManagedCluster
	addons         []addonv1alpha1.ManagedClusterAddOn
	works          []workv1.ManifestWork
	orphanedWorks  []string
}

func (c *reimportTestClient) GetManagedCluster(_ context.Context, name string) (*clusterv1.ManagedCluster, error) {
	if c.managedCluster == nil {
		return nil, apierrors.NewNotFound(clusterv1.Resource("managedclusters"), name)
	}
	return c.managedCluster.DeepCopy(), nil
}
func (c *reimportTestClient) UpdateManagedClusterHosting(_ context.Context, _ string, uid, _ string, replacement string, updateAddonHosting bool) error {
	if string(c.managedCluster.UID) != uid {
		return fmt.Errorf("ManagedCluster was recreated")
	}
	c.managedCluster.Annotations[importHostingAnnotation] = replacement
	if updateAddonHosting {
		c.managedCluster.Annotations[addonHostingAnnotation] = replacement
	}
	return nil
}
func (c *reimportTestClient) ListManagedClusterAddons(_ context.Context, namespace string) ([]addonv1alpha1.ManagedClusterAddOn, error) {
	addons := make([]addonv1alpha1.ManagedClusterAddOn, 0, len(c.addons))
	for _, addon := range c.addons {
		if addon.Namespace == namespace {
			addons = append(addons, *addon.DeepCopy())
		}
	}
	return addons, nil
}
func (c *reimportTestClient) UpdateManagedClusterAddonHosting(_ context.Context, namespace, name, uid, currentHosting, replacement string) error {
	for i := range c.addons {
		addon := &c.addons[i]
		if addon.Namespace == namespace && addon.Name == name && string(addon.UID) == uid && addon.Annotations[addonHostingAnnotation] == currentHosting {
			addon.Annotations[addonHostingAnnotation] = replacement
			return nil
		}
	}
	return fmt.Errorf("ManagedClusterAddon %s/%s changed", namespace, name)
}

func (c *reimportTestClient) GetManifestWork(_ context.Context, namespace, name string) (*workv1.ManifestWork, error) {
	for i := range c.works {
		if c.works[i].Namespace == namespace && c.works[i].Name == name {
			return c.works[i].DeepCopy(), nil
		}
	}
	return nil, apierrors.NewNotFound(workv1.Resource("manifestworks"), name)
}
func (c *reimportTestClient) ListManifestWorks(_ context.Context, namespace string, options ...internalclient.ManifestWorkListOption) ([]workv1.ManifestWork, error) {
	labels := internalclient.ManifestWorkLabels(options...)
	var works []workv1.ManifestWork
	for _, work := range c.works {
		if namespace != "" && work.Namespace != namespace {
			continue
		}
		matches := true
		for key, value := range labels {
			if work.Labels[key] != value {
				matches = false
			}
		}
		if matches {
			works = append(works, *work.DeepCopy())
		}
	}
	return works, nil
}
func (*reimportTestClient) CreateManifestWork(context.Context, *workv1.ManifestWork) error {
	return nil
}
func (*reimportTestClient) UpdateManifestWork(context.Context, *workv1.ManifestWork) error {
	return nil
}
func (c *reimportTestClient) OrphanDeleteManifestWork(_ context.Context, namespace, name, uid string) error {
	for i := range c.works {
		work := c.works[i]
		if work.Namespace == namespace && work.Name == name && string(work.UID) == uid {
			c.orphanedWorks = append(c.orphanedWorks, namespace+"/"+name)
			c.works = append(c.works[:i], c.works[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("ManifestWork %s/%s changed", namespace, name)
}

type reimportTestOCMClient struct {
	replacement *cmv1.Cluster
	client      internalclient.ServiceClusterClient
}

func (*reimportTestOCMClient) Resolve(context.Context, string) (internalclient.ClusterInfo, error) {
	return internalclient.ClusterInfo{HCPClusterID: "managed"}, nil
}
func (*reimportTestOCMClient) ResolveServiceCluster(context.Context, string) (internalclient.ServiceClusterInfo, error) {
	return internalclient.ServiceClusterInfo{ID: "service-id", Name: "service-name"}, nil
}
func (c *reimportTestOCMClient) Cluster(string) (*cmv1.Cluster, error) { return c.replacement, nil }
func (c *reimportTestOCMClient) ServiceCluster(string, string) (internalclient.ServiceClusterClient, error) {
	return c.client, nil
}

func TestReimportUpdatesManagedClusterForReplacement(t *testing.T) {
	replacement, err := cmv1.NewCluster().Name("replacement-name").Build()
	require.NoError(t, err)
	client := &reimportTestClient{managedCluster: &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "managed", UID: types.UID("original"), Labels: map[string]string{"keep": "label"}, Annotations: hostedAnnotations("old-management-cluster")}}}
	manager := NewDefaultReimportManager(&reimportTestOCMClient{replacement: replacement, client: client})
	plan, err := manager.PrepareReimport(context.Background(), ReimportRequest{ClusterID: "cluster", ReplacementMC: "replacement", Reason: "recovery", Timeout: time.Second})
	require.NoError(t, err)
	require.NoError(t, manager.Reimport(context.Background(), plan, time.Second))
	assert.Equal(t, "replacement-name", client.managedCluster.Annotations[importHostingAnnotation])
	assert.Equal(t, "replacement-name", client.managedCluster.Annotations[addonHostingAnnotation])
	assert.Equal(t, map[string]string{"keep": "label"}, client.managedCluster.Labels)
}

func TestReimportPreservesManagedClusterFinalizers(t *testing.T) {
	replacement, err := cmv1.NewCluster().Name("replacement-name").Build()
	require.NoError(t, err)
	client := &reimportTestClient{managedCluster: &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "managed", UID: types.UID("original"), Finalizers: []string{"managedclusterinfo.finalizers.open-cluster-management.io"}, Annotations: hostedAnnotations("old-management-cluster")}}}
	manager := NewDefaultReimportManager(&reimportTestOCMClient{replacement: replacement, client: client})
	plan, err := manager.PrepareReimport(context.Background(), ReimportRequest{ClusterID: "cluster", ReplacementMC: "replacement", Reason: "recovery", Timeout: time.Second})
	require.NoError(t, err)
	require.NoError(t, manager.Reimport(context.Background(), plan, time.Second))
	assert.Equal(t, []string{"managedclusterinfo.finalizers.open-cluster-management.io"}, client.managedCluster.Finalizers)
}

func TestReimportRejectsRecreatedManagedCluster(t *testing.T) {
	replacement, err := cmv1.NewCluster().Name("replacement-name").Build()
	require.NoError(t, err)
	client := &reimportTestClient{managedCluster: &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "managed", UID: types.UID("original"), Annotations: hostedAnnotations("old-management-cluster")}}}
	manager := NewDefaultReimportManager(&reimportTestOCMClient{replacement: replacement, client: client})
	plan, err := manager.PrepareReimport(context.Background(), ReimportRequest{ClusterID: "cluster", ReplacementMC: "replacement", Reason: "recovery", Timeout: time.Second})
	require.NoError(t, err)
	client.managedCluster.UID = types.UID("recreated")
	require.EqualError(t, manager.Reimport(context.Background(), plan, time.Second), "ManagedCluster managed was recreated; rerun to review it")
	assert.Equal(t, "old-management-cluster", client.managedCluster.Annotations[importHostingAnnotation])
}

func TestReimportUpdatesAddonHostingAnnotation(t *testing.T) {
	replacement, err := cmv1.NewCluster().Name("replacement-name").Build()
	require.NoError(t, err)
	client := &reimportTestClient{
		managedCluster: &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "managed", UID: types.UID("managed"), Annotations: hostedAnnotations("old-management-cluster")}},
		addons:         []addonv1alpha1.ManagedClusterAddOn{{ObjectMeta: metav1.ObjectMeta{Namespace: "managed", Name: "work-manager", UID: types.UID("addon"), Finalizers: []string{"example/finalizer"}, Annotations: map[string]string{addonHostingAnnotation: "old-management-cluster", "keep": "annotation"}}}},
	}
	manager := NewDefaultReimportManager(&reimportTestOCMClient{replacement: replacement, client: client})
	plan, err := manager.PrepareReimport(context.Background(), ReimportRequest{ClusterID: "cluster", ReplacementMC: "replacement", Reason: "recovery", Timeout: time.Second})
	require.NoError(t, err)
	require.NoError(t, manager.Reimport(context.Background(), plan, time.Second))
	assert.Equal(t, "replacement-name", client.addons[0].Annotations[addonHostingAnnotation])
	assert.Equal(t, "annotation", client.addons[0].Annotations["keep"])
	assert.Equal(t, []string{"example/finalizer"}, client.addons[0].Finalizers)
}

func TestReimportRejectsChangedAddonAfterPreview(t *testing.T) {
	replacement, err := cmv1.NewCluster().Name("replacement-name").Build()
	require.NoError(t, err)
	client := &reimportTestClient{
		managedCluster: &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "managed", UID: types.UID("managed"), Annotations: hostedAnnotations("old-management-cluster")}},
		addons:         []addonv1alpha1.ManagedClusterAddOn{{ObjectMeta: metav1.ObjectMeta{Namespace: "managed", Name: "work-manager", UID: types.UID("original"), Annotations: map[string]string{addonHostingAnnotation: "old-management-cluster"}}}},
	}
	manager := NewDefaultReimportManager(&reimportTestOCMClient{replacement: replacement, client: client})
	plan, err := manager.PrepareReimport(context.Background(), ReimportRequest{ClusterID: "cluster", ReplacementMC: "replacement", Reason: "recovery", Timeout: time.Second})
	require.NoError(t, err)
	client.addons[0].Annotations[addonHostingAnnotation] = "changed-management-cluster"
	assert.Error(t, manager.Reimport(context.Background(), plan, time.Second))
	assert.Equal(t, "changed-management-cluster", client.addons[0].Annotations[addonHostingAnnotation])
}

func TestReimportOrphansOnlyOldHostAddonWorks(t *testing.T) {
	replacement, err := cmv1.NewCluster().Name("replacement-name").Build()
	require.NoError(t, err)
	labels := map[string]string{
		"open-cluster-management.io/addon-name":      "governance-policy-framework",
		"open-cluster-management.io/addon-namespace": "managed",
	}
	client := &reimportTestClient{
		managedCluster: &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "managed", UID: types.UID("managed"), Annotations: hostedAnnotations("old-management-cluster")}},
		works: []workv1.ManifestWork{
			{ObjectMeta: metav1.ObjectMeta{Namespace: "old-management-cluster", Name: "old", UID: "old", Labels: labels}},
			{ObjectMeta: metav1.ObjectMeta{Namespace: "replacement-name", Name: "replacement", UID: "replacement", Labels: labels}},
		},
	}
	manager := NewDefaultReimportManager(&reimportTestOCMClient{replacement: replacement, client: client})
	plan, err := manager.PrepareReimport(context.Background(), ReimportRequest{ClusterID: "cluster", ReplacementMC: "replacement", Reason: "recovery", Timeout: time.Second})
	require.NoError(t, err)
	assert.Equal(t, []string{"old-management-cluster/old"}, plan.OldAddonManifestWorks)
	require.NoError(t, manager.Reimport(context.Background(), plan, time.Second))
	assert.Equal(t, []string{"old-management-cluster/old"}, client.orphanedWorks)
	assert.Len(t, client.works, 1)
	assert.Equal(t, "replacement-name", client.works[0].Namespace)
}

func TestPrepareReimportRejectsNonPositiveTimeout(t *testing.T) {
	manager := NewDefaultReimportManager(&reimportTestOCMClient{})
	_, err := manager.PrepareReimport(context.Background(), ReimportRequest{ClusterID: "cluster", ReplacementMC: "replacement", Reason: "recovery"})
	require.EqualError(t, err, "timeout must be positive")
}

func hostedAnnotations(hosting string) map[string]string {
	annotations := map[string]string{addonHostingAnnotation: hosting, importHostingAnnotation: hosting}
	for key, value := range requiredHostedModeAnnotations {
		annotations[key] = value
	}
	return annotations
}
