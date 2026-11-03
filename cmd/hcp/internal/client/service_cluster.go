package client

import (
	"context"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/types"
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
