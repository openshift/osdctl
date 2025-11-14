package resize

import (
	"context"
	"fmt"
	"testing"

	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGetNextSize(t *testing.T) {
	availableSizes := []clusterSize{
		{Name: "m5xl", Criteria: sizeCriteria{From: 0, To: 60}},
		{Name: "m52xl", Criteria: sizeCriteria{From: 61, To: 150}},
		{Name: "m54xl", Criteria: sizeCriteria{From: 151, To: 221}},
		{Name: "r54xl", Criteria: sizeCriteria{From: 222, To: 321}},
		{Name: "r58xl", Criteria: sizeCriteria{From: 322, To: 0}},
	}

	tests := []struct {
		name          string
		currentSize   string
		expected      string
		expectErr     bool
		errorContains string
	}{
		{
			name:        "first to second size",
			currentSize: "m5xl",
			expected:    "m52xl",
			expectErr:   false,
		},
		{
			name:        "middle to next size",
			currentSize: "m52xl",
			expected:    "m54xl",
			expectErr:   false,
		},
		{
			name:        "second to last to last",
			currentSize: "r54xl",
			expected:    "r58xl",
			expectErr:   false,
		},
		{
			name:          "already at largest",
			currentSize:   "r58xl",
			expectErr:     true,
			errorContains: "already the largest available size",
		},
		{
			name:          "invalid current size",
			currentSize:   "invalid",
			expectErr:     true,
			errorContains: "not found in available sizes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &requestServingNodesOpts{}
			result, err := r.getNextSize(tt.currentSize, availableSizes)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestIsValidSize(t *testing.T) {
	availableSizes := []clusterSize{
		{Name: "m5xl", Criteria: sizeCriteria{From: 0, To: 60}},
		{Name: "m52xl", Criteria: sizeCriteria{From: 61, To: 150}},
		{Name: "m54xl", Criteria: sizeCriteria{From: 151, To: 221}},
	}

	tests := []struct {
		name     string
		size     string
		expected bool
	}{
		{
			name:     "valid size - first",
			size:     "m5xl",
			expected: true,
		},
		{
			name:     "valid size - middle",
			size:     "m52xl",
			expected: true,
		},
		{
			name:     "valid size - last",
			size:     "m54xl",
			expected: true,
		},
		{
			name:     "invalid size",
			size:     "invalid",
			expected: false,
		},
		{
			name:     "empty size",
			size:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &requestServingNodesOpts{}
			result := r.isValidSize(tt.size, availableSizes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSizeNames(t *testing.T) {
	tests := []struct {
		name     string
		sizes    []clusterSize
		expected []string
	}{
		{
			name: "multiple sizes",
			sizes: []clusterSize{
				{Name: "m5xl", Criteria: sizeCriteria{From: 0, To: 60}},
				{Name: "m52xl", Criteria: sizeCriteria{From: 61, To: 150}},
				{Name: "m54xl", Criteria: sizeCriteria{From: 151, To: 221}},
			},
			expected: []string{"m5xl", "m52xl", "m54xl"},
		},
		{
			name:     "empty sizes",
			sizes:    []clusterSize{},
			expected: []string{},
		},
		{
			name: "single size",
			sizes: []clusterSize{
				{Name: "m5xl", Criteria: sizeCriteria{From: 0, To: 60}},
			},
			expected: []string{"m5xl"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &requestServingNodesOpts{}
			result := r.getSizeNames(tt.sizes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindHostedCluster(t *testing.T) {
	tests := []struct {
		name          string
		clusterID     string
		mockItems     []hypershiftv1beta1.HostedCluster
		mockError     error
		expectErr     bool
		errorContains string
		expectedNS    string
		expectedName  string
	}{
		{
			name:      "single hosted cluster found",
			clusterID: "test-cluster-id",
			mockItems: []hypershiftv1beta1.HostedCluster{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hc",
						Namespace: "ocm-production-abc123",
						Labels: map[string]string{
							"api.openshift.com/id": "test-cluster-id",
						},
					},
				},
			},
			expectErr:    false,
			expectedNS:   "ocm-production-abc123",
			expectedName: "test-hc",
		},
		{
			name:          "no hosted cluster found",
			clusterID:     "test-cluster-id",
			mockItems:     []hypershiftv1beta1.HostedCluster{},
			expectErr:     true,
			errorContains: "no hostedcluster found",
		},
		{
			name:      "multiple hosted clusters found",
			clusterID: "test-cluster-id",
			mockItems: []hypershiftv1beta1.HostedCluster{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hc-1",
						Namespace: "ocm-production-abc123",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hc-2",
						Namespace: "ocm-production-def456",
					},
				},
			},
			expectErr:     true,
			errorContains: "found 2 hostedclusters",
		},
		{
			name:          "list error",
			clusterID:     "test-cluster-id",
			mockError:     fmt.Errorf("api server error"),
			expectErr:     true,
			errorContains: "failed to list hostedclusters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockClient{}
			mockClient.On("List", mock.Anything, mock.AnythingOfType("*v1beta1.HostedClusterList"), mock.Anything).
				Return(tt.mockError).
				Run(func(args mock.Arguments) {
					if tt.mockItems != nil {
						list := args.Get(1).(*hypershiftv1beta1.HostedClusterList)
						list.Items = tt.mockItems
					}
				})

			r := &requestServingNodesOpts{
				mgmtClient: mockClient,
			}

			result, err := r.findHostedCluster(context.Background(), tt.clusterID)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedNS, result.Namespace)
				assert.Equal(t, tt.expectedName, result.Name)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetAvailableSizes(t *testing.T) {
	tests := []struct {
		name          string
		mockObject    *unstructured.Unstructured
		mockError     error
		expected      []clusterSize
		expectErr     bool
		errorContains string
	}{
		{
			name: "valid cluster sizing configuration",
			mockObject: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "scheduling.hypershift.openshift.io/v1alpha1",
					"kind":       "ClusterSizingConfiguration",
					"metadata": map[string]interface{}{
						"name": "cluster",
					},
					"spec": map[string]interface{}{
						"sizes": []interface{}{
							map[string]interface{}{
								"name": "m5xl",
								"criteria": map[string]interface{}{
									"from": int64(0),
									"to":   int64(60),
								},
							},
							map[string]interface{}{
								"name": "m52xl",
								"criteria": map[string]interface{}{
									"from": int64(61),
									"to":   int64(150),
								},
							},
						},
					},
				},
			},
			expected: []clusterSize{
				{Name: "m5xl", Criteria: sizeCriteria{From: 0, To: 60}},
				{Name: "m52xl", Criteria: sizeCriteria{From: 61, To: 150}},
			},
			expectErr: false,
		},
		{
			name: "no sizes in configuration",
			mockObject: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"sizes": []interface{}{},
					},
				},
			},
			expectErr:     true,
			errorContains: "no cluster sizes found",
		},
		{
			name:          "get error",
			mockError:     fmt.Errorf("api server error"),
			expectErr:     true,
			errorContains: "failed to get cluster sizing configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockClient{}
			mockClient.On("Get", mock.Anything, mock.Anything, mock.AnythingOfType("*unstructured.Unstructured"), mock.Anything).
				Return(tt.mockError).
				Run(func(args mock.Arguments) {
					if tt.mockObject != nil {
						obj := args.Get(2).(*unstructured.Unstructured)
						obj.Object = tt.mockObject.Object
					}
				})

			r := &requestServingNodesOpts{
				mgmtClient: mockClient,
			}

			result, err := r.getAvailableSizes(context.Background())

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tt.expected), len(result))
				for i, expected := range tt.expected {
					assert.Equal(t, expected.Name, result[i].Name)
					assert.Equal(t, expected.Criteria.From, result[i].Criteria.From)
					assert.Equal(t, expected.Criteria.To, result[i].Criteria.To)
				}
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestApplyClusterSizeOverride(t *testing.T) {
	tests := []struct {
		name          string
		hostedCluster *hypershiftv1beta1.HostedCluster
		targetSize    string
		patchError    error
		expectErr     bool
		errorContains string
	}{
		{
			name: "successful annotation - no existing annotations",
			hostedCluster: &hypershiftv1beta1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace",
				},
			},
			targetSize: "m54xl",
			expectErr:  false,
		},
		{
			name: "successful annotation - existing annotations",
			hostedCluster: &hypershiftv1beta1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"existing": "annotation",
					},
				},
			},
			targetSize: "m54xl",
			expectErr:  false,
		},
		{
			name: "patch error",
			hostedCluster: &hypershiftv1beta1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace",
				},
			},
			targetSize:    "m54xl",
			patchError:    fmt.Errorf("api server error"),
			expectErr:     true,
			errorContains: "failed to patch hostedcluster annotation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockClient{}
			mockClient.On("Patch", mock.Anything, mock.AnythingOfType("*v1beta1.HostedCluster"), mock.Anything, mock.Anything).
				Return(tt.patchError)

			r := &requestServingNodesOpts{
				mgmtClientAdmin: mockClient,
			}

			err := r.applyClusterSizeOverride(context.Background(), tt.hostedCluster, tt.targetSize)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				// Verify the annotation was set
				assert.Equal(t, tt.targetSize, tt.hostedCluster.Annotations["hypershift.openshift.io/cluster-size-override"])
			}

			mockClient.AssertExpectations(t)
		})
	}
}
