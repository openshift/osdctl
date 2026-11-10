package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// serviceClusterClient provides the ManifestWork operations available on an
// initialized service-cluster connection.
type serviceClusterClient struct{ client client.Client }

type ManifestWorkListConfig struct {
	labels map[string]string
}

func (c *ManifestWorkListConfig) Options(opts ...ManifestWorkListOption) {
	for _, opt := range opts {
		opt.ConfigureManifestWorkList(c)
	}
}

// ManifestWorkListOption configures a ManifestWork list operation.
type ManifestWorkListOption interface {
	ConfigureManifestWorkList(*ManifestWorkListConfig)
}

type manifestWorkLabel struct {
	key   string
	value string
}

// WithManifestWorkLabel filters ManifestWorks by a label key and value.
func WithManifestWorkLabel(key, value string) ManifestWorkListOption {
	return manifestWorkLabel{key: key, value: value}
}

func (l manifestWorkLabel) ConfigureManifestWorkList(config *ManifestWorkListConfig) {
	if config.labels == nil {
		config.labels = make(map[string]string)
	}
	config.labels[l.key] = l.value
}

// ManifestWorkLabels returns the label filters configured by options.
func ManifestWorkLabels(options ...ManifestWorkListOption) map[string]string {
	config := ManifestWorkListConfig{}
	config.Options(options...)
	return config.labels
}

func (c *serviceClusterClient) GetManifestWork(ctx context.Context, namespace, name string) (*workv1.ManifestWork, error) {
	work := &workv1.ManifestWork{}
	if err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, work); err != nil {
		return nil, err
	}
	return work, nil
}

func (c *serviceClusterClient) ListManifestWorks(ctx context.Context, namespace string, options ...ManifestWorkListOption) ([]workv1.ManifestWork, error) {
	list := &workv1.ManifestWorkList{}
	listOptions := []client.ListOption{client.InNamespace(namespace)}
	if labels := ManifestWorkLabels(options...); len(labels) > 0 {
		listOptions = append(listOptions, client.MatchingLabels(labels))
	}
	if err := c.client.List(ctx, list, listOptions...); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool { return list.Items[i].Name < list.Items[j].Name })
	return list.Items, nil
}

func (c *serviceClusterClient) CreateManifestWork(ctx context.Context, work *workv1.ManifestWork) error {
	if err := c.client.Create(ctx, work); err != nil {
		return fmt.Errorf("creating ManifestWork %s/%s: %w", work.Namespace, work.Name, err)
	}
	return nil
}

func (c *serviceClusterClient) UpdateManifestWork(ctx context.Context, work *workv1.ManifestWork) error {
	if err := c.client.Update(ctx, work); err != nil {
		return fmt.Errorf("updating ManifestWork %s/%s: %w", work.Namespace, work.Name, err)
	}
	return nil
}

// OrphanDeleteManifestWork preserves resources applied by the ManifestWork.
func (c *serviceClusterClient) OrphanDeleteManifestWork(ctx context.Context, namespace, name, uid string) error {
	work, err := c.GetManifestWork(ctx, namespace, name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("getting ManifestWork %s/%s before orphan deletion: %w", namespace, name, err)
	}
	if string(work.UID) != uid {
		return fmt.Errorf("ManifestWork %s/%s was recreated", namespace, name)
	}
	if work.DeletionTimestamp != nil {
		if work.Spec.DeleteOption == nil || work.Spec.DeleteOption.PropagationPolicy != workv1.DeletePropagationPolicyTypeOrphan {
			return fmt.Errorf("ManifestWork %s/%s is already deleting without Orphan policy", namespace, name)
		}
		return nil
	}
	patch, err := json.Marshal([]map[string]interface{}{
		{"op": "test", "path": "/metadata/uid", "value": uid},
		{"op": "test", "path": "/metadata/resourceVersion", "value": work.ResourceVersion},
		{"op": "add", "path": "/spec/deleteOption", "value": workv1.DeleteOption{PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan}},
	})
	if err != nil {
		return fmt.Errorf("encoding ManifestWork orphan policy patch: %w", err)
	}
	if err := c.client.Patch(ctx, work, client.RawPatch(types.JSONPatchType, patch)); err != nil {
		return fmt.Errorf("setting ManifestWork %s/%s Orphan policy: %w", namespace, name, err)
	}
	if err := c.client.Delete(ctx, work, client.Preconditions{UID: &work.UID}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("deleting ManifestWork %s/%s with Orphan policy: %w", namespace, name, err)
	}
	return nil
}

func (c *serviceClusterClient) GetManagedCluster(ctx context.Context, name string) (*clusterv1.ManagedCluster, error) {
	managedCluster := &clusterv1.ManagedCluster{}
	if err := c.client.Get(ctx, types.NamespacedName{Name: name}, managedCluster); err != nil {
		return nil, err
	}
	return managedCluster, nil
}

// UpdateManagedClusterHosting atomically replaces the import hosting annotation
// and an existing addon hosting annotation only when the resource is unchanged.
func (c *serviceClusterClient) UpdateManagedClusterHosting(ctx context.Context, name, uid, resourceVersion, replacementMC string, updateAddonHosting bool) error {
	operations := []map[string]string{
		{"op": "test", "path": "/metadata/uid", "value": uid},
		{"op": "test", "path": "/metadata/resourceVersion", "value": resourceVersion},
		{"op": "replace", "path": "/metadata/annotations/import.open-cluster-management.io~1hosting-cluster-name", "value": replacementMC},
	}
	if updateAddonHosting {
		operations = append(operations, map[string]string{"op": "replace", "path": "/metadata/annotations/addon.open-cluster-management.io~1hosting-cluster-name", "value": replacementMC})
	}
	patch, err := json.Marshal(operations)
	if err != nil {
		return fmt.Errorf("encoding ManagedCluster hosting annotation patch: %w", err)
	}
	managedCluster := &clusterv1.ManagedCluster{}
	managedCluster.Name = name
	if err := c.client.Patch(ctx, managedCluster, client.RawPatch(types.JSONPatchType, patch)); err != nil {
		return fmt.Errorf("patching ManagedCluster %s hosting annotations: %w", name, err)
	}
	return nil
}

func (c *serviceClusterClient) ListManagedClusterAddons(ctx context.Context, namespace string) ([]addonv1alpha1.ManagedClusterAddOn, error) {
	list := &addonv1alpha1.ManagedClusterAddOnList{}
	if err := c.client.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool { return list.Items[i].Name < list.Items[j].Name })
	return list.Items, nil
}

// UpdateManagedClusterAddonHosting atomically replaces an existing hosting annotation
// if the add-on has not been recreated or moved to another hosting cluster.
func (c *serviceClusterClient) UpdateManagedClusterAddonHosting(ctx context.Context, namespace, name, uid, currentHostingMC, replacementMC string) error {
	patch, err := json.Marshal([]map[string]string{
		{"op": "test", "path": "/metadata/uid", "value": uid},
		{"op": "test", "path": "/metadata/annotations/addon.open-cluster-management.io~1hosting-cluster-name", "value": currentHostingMC},
		{"op": "replace", "path": "/metadata/annotations/addon.open-cluster-management.io~1hosting-cluster-name", "value": replacementMC},
	})
	if err != nil {
		return fmt.Errorf("encoding ManagedClusterAddon hosting annotation patch: %w", err)
	}
	addon := &addonv1alpha1.ManagedClusterAddOn{}
	addon.Namespace = namespace
	addon.Name = name
	if err := c.client.Patch(ctx, addon, client.RawPatch(types.JSONPatchType, patch)); err != nil {
		return fmt.Errorf("patching ManagedClusterAddon %s/%s hosting annotation: %w", namespace, name, err)
	}
	return nil
}
