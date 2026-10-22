package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
