package resize

import (
	"context"
	"fmt"
	"testing"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// newTestCluster assembles a *cmv1.Cluster while handling the error to help out with inline test-case generation
func newTestCluster(t *testing.T, cb *cmv1.ClusterBuilder) *cmv1.Cluster {
	cluster, err := cb.Build()
	if err != nil {
		t.Fatalf("failed to build cluster: %s", err)
	}

	return cluster
}

func TestResize_embiggenMachinePool(t *testing.T) {
	tests := []struct {
		name      string
		cluster   *cmv1.Cluster
		mp        *hivev1.MachinePool
		override  string
		expected  string
		expectErr bool
	}{
		{
			name:    "AWS r5.xlarge --> r5.2xlarge",
			cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws"))),
			mp: &hivev1.MachinePool{
				Spec: hivev1.MachinePoolSpec{
					Platform: hivev1.MachinePoolPlatform{
						AWS: &hivev1aws.MachinePoolPlatform{
							InstanceType: "r5.xlarge",
						},
					},
				},
			},
			expected:  "r5.2xlarge",
			expectErr: false,
		},
		{
			name:    "GCP custom-4-32768-ext --> custom-8-65536-ext",
			cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("gcp"))),
			mp: &hivev1.MachinePool{
				Spec: hivev1.MachinePoolSpec{
					Platform: hivev1.MachinePoolPlatform{
						GCP: &hivev1gcp.MachinePool{
							InstanceType: "custom-4-32768-ext",
						},
					},
				},
			},
			expected:  "custom-8-65536-ext",
			expectErr: false,
		},
		{
			name:    "AWS r5.2xlarge --> r5.xlarge with override",
			cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws"))),
			mp: &hivev1.MachinePool{
				Spec: hivev1.MachinePoolSpec{
					Platform: hivev1.MachinePoolPlatform{
						AWS: &hivev1aws.MachinePoolPlatform{
							InstanceType: "r5.2xlarge",
						},
					},
				},
			},
			override:  "r5.xlarge",
			expected:  "r5.xlarge",
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := &Infra{
				cluster:      test.cluster,
				instanceType: test.override,
			}
			actual, err := r.embiggenMachinePool(test.mp)
			if err != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %v", err)
				}
			} else {
				if test.expectErr {
					t.Error("expected err, got nil")
				}

				actualInstanceType, err := getInstanceType(actual)
				if err != nil {
					t.Error(err)
				}

				if test.expected != actualInstanceType {
					t.Errorf("expected: %s, got %s", test.expected, actualInstanceType)
				}
			}
		})
	}
}

func TestConvertProviderIDtoInstanceID(t *testing.T) {
	tests := []struct {
		providerID string
		expected   string
	}{
		{
			providerID: "aws:///us-east-1a/i-0a1b2c3d4e5f6g7h8",
			expected:   "i-0a1b2c3d4e5f6g7h8",
		},
		{
			providerID: "gce://some-string/europe-west4-a/my-cluster-name-n65hp-infra-a-4fbrd",
			expected:   "my-cluster-name-n65hp-infra-a-4fbrd",
		},
	}

	for _, test := range tests {
		t.Run(test.providerID, func(t *testing.T) {
			actual := convertProviderIDtoInstanceID(test.providerID)
			if test.expected != actual {
				t.Errorf("expected: %s, got %s", test.expected, actual)
			}
		})
	}
}

func TestSkipError(t *testing.T) {
	tests := []struct {
		name     string
		result   result
		msg      string
		expected bool
	}{
		{
			name: "no error",
			result: result{
				condition: true,
				err:       nil,
			},
			msg:      "test message",
			expected: true,
		},
		{
			name: "with error",
			result: result{
				condition: false,
				err:       fmt.Errorf("test error"),
			},
			msg:      "test message",
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := skipError(test.result, test.msg)
			if err != nil {
				t.Errorf("expected nil error, got %v", err)
			}
			if actual != test.expected {
				t.Errorf("expected condition %v, got %v", test.expected, actual)
			}
		})
	}
}

func TestNodesMatchExpectedCount(t *testing.T) {
	tests := []struct {
		name          string
		labelSelector labels.Selector
		expectedCount int
		mockNodeList  *corev1.NodeList
		mockListError error
		expectedMatch bool
		expectedError error
	}{
		{
			name:          "matching count",
			labelSelector: labels.NewSelector(),
			expectedCount: 2,
			mockNodeList: &corev1.NodeList{
				Items: []corev1.Node{
					{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
				},
			},
			expectedMatch: true,
		},
		{
			name:          "non-matching count",
			labelSelector: labels.NewSelector(),
			expectedCount: 2,
			mockNodeList: &corev1.NodeList{
				Items: []corev1.Node{
					{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
				},
			},
			expectedMatch: false,
		},
		{
			name:          "list error",
			labelSelector: labels.NewSelector(),
			expectedCount: 2,
			mockListError: fmt.Errorf("list error"),
			expectedError: fmt.Errorf("list error"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create mock client
			mockClient := &MockClient{}
			mockClient.On("List", mock.Anything, mock.Anything, mock.Anything).
				Return(test.mockListError).
				Run(func(args mock.Arguments) {
					if test.mockNodeList != nil {
						arg := args.Get(1).(*corev1.NodeList)
						*arg = *test.mockNodeList
					}
				})

			// Create Infra instance with mock client
			r := &Infra{
				client: mockClient,
			}

			// Call the function
			match, err := r.nodesMatchExpectedCount(context.Background(), test.labelSelector, test.expectedCount)

			// Verify results
			if test.expectedError != nil {
				if err == nil || err.Error() != test.expectedError.Error() {
					t.Errorf("expected error %v, got %v", test.expectedError, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if match != test.expectedMatch {
					t.Errorf("expected match %v, got %v", test.expectedMatch, match)
				}
			}

			// Verify mock was called correctly
			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetInfraMachinePool(t *testing.T) {
	testNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
			Labels: map[string]string{
				"api.openshift.com/id": "test-cluster",
			},
		},
	}

	testMachinePool := &hivev1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-infra",
			Namespace: "test-namespace",
		},
		Spec: hivev1.MachinePoolSpec{
			Name: "infra",
			Platform: hivev1.MachinePoolPlatform{
				AWS: &hivev1aws.MachinePoolPlatform{
					InstanceType: "r5.xlarge",
				},
			},
		},
	}

	mockHive := &MockClient{}
	firstCall := mockHive.On("List", mock.Anything, mock.MatchedBy(func(obj interface{}) bool {
		_, ok := obj.(*corev1.NamespaceList)
		return ok
	}), mock.Anything)
	firstCall.Return(nil).Run(func(args mock.Arguments) {
		nsList := args.Get(1).(*corev1.NamespaceList)
		nsList.Items = []corev1.Namespace{*testNamespace}
	})

	secondCall := mockHive.On("List", mock.Anything, mock.MatchedBy(func(obj interface{}) bool {
		_, ok := obj.(*hivev1.MachinePoolList)
		return ok
	}), mock.Anything)
	secondCall.Return(nil).Run(func(args mock.Arguments) {
		mpList := args.Get(1).(*hivev1.MachinePoolList)
		mpList.Items = []hivev1.MachinePool{*testMachinePool}
	})

	infra := &Infra{
		clusterId: "test-cluster",
		hive:      mockHive,
	}

	mp, err := infra.getInfraMachinePool(context.Background())

	assert.NoError(t, err)
	assert.NotNil(t, mp)
	assert.Equal(t, "infra", mp.Spec.Name)
	assert.Equal(t, "r5.xlarge", mp.Spec.Platform.AWS.InstanceType)
	mockHive.AssertExpectations(t)
}

func TestGetInfraMachinePoolNoNamespace(t *testing.T) {
	// Create mock client
	mockHive := &MockClient{}

	// Set up mock expectations for namespace list - empty list
	firstCall := mockHive.On("List", mock.Anything, mock.MatchedBy(func(obj interface{}) bool {
		_, ok := obj.(*corev1.NamespaceList)
		return ok
	}), mock.Anything)
	firstCall.Return(nil).Run(func(args mock.Arguments) {
		nsList := args.Get(1).(*corev1.NamespaceList)
		nsList.Items = []corev1.Namespace{} // Empty list
	})

	// Create Infra instance
	infra := &Infra{
		clusterId: "test-cluster",
		hive:      mockHive,
	}

	// Call the function
	mp, err := infra.getInfraMachinePool(context.Background())

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected 1 namespace, found 0 namespaces with tag: api.openshift.com/id=test-cluster")
	assert.Nil(t, mp)

	// Verify mock was called correctly
	mockHive.AssertExpectations(t)
}

func TestGetInfraMachinePoolNoInfraPool(t *testing.T) {
	// Create a test namespace
	testNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
			Labels: map[string]string{
				"api.openshift.com/id": "test-cluster",
			},
		},
	}

	// Create a test machine pool (worker, not infra)
	testMachinePool := &hivev1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-worker",
			Namespace: "test-namespace",
		},
		Spec: hivev1.MachinePoolSpec{
			Name: "worker", // Not "infra"
			Platform: hivev1.MachinePoolPlatform{
				AWS: &hivev1aws.MachinePoolPlatform{
					InstanceType: "r5.xlarge",
				},
			},
		},
	}

	// Create mock client
	mockHive := &MockClient{}

	// Set up mock expectations for namespace list - first call
	firstCall := mockHive.On("List", mock.Anything, mock.MatchedBy(func(obj interface{}) bool {
		_, ok := obj.(*corev1.NamespaceList)
		return ok
	}), mock.Anything)
	firstCall.Return(nil).Run(func(args mock.Arguments) {
		nsList := args.Get(1).(*corev1.NamespaceList)
		nsList.Items = []corev1.Namespace{*testNamespace}
	})

	// Set up mock expectations for machine pool list - second call
	secondCall := mockHive.On("List", mock.Anything, mock.MatchedBy(func(obj interface{}) bool {
		_, ok := obj.(*hivev1.MachinePoolList)
		return ok
	}), mock.Anything)
	secondCall.Return(nil).Run(func(args mock.Arguments) {
		mpList := args.Get(1).(*hivev1.MachinePoolList)
		mpList.Items = []hivev1.MachinePool{*testMachinePool}
	})

	// Create Infra instance
	infra := &Infra{
		clusterId: "test-cluster",
		hive:      mockHive,
	}

	// Call the function
	mp, err := infra.getInfraMachinePool(context.Background())

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "did not find the infra machinepool in namespace: test-namespace")
	assert.Nil(t, mp)

	// Verify mock was called correctly
	mockHive.AssertExpectations(t)
}
