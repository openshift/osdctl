package infra

import (
	"context"
	"fmt"
	"testing"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MockClient is a mock implementation of the client.Client interface
type MockClient struct {
	mock.Mock
}

func (m *MockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	args := m.Called(ctx, list, opts)
	return args.Error(0)
}

func (m *MockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	args := m.Called(ctx, key, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	args := m.Called(ctx, obj, patch, opts)
	return args.Error(0)
}

func (m *MockClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	args := m.Called(obj)
	return args.Get(0).(schema.GroupVersionKind), args.Error(1)
}

func (m *MockClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	args := m.Called(obj)
	return args.Bool(0), args.Error(1)
}

func (m *MockClient) RESTMapper() meta.RESTMapper {
	args := m.Called()
	return args.Get(0).(meta.RESTMapper)
}

func (m *MockClient) Scheme() *runtime.Scheme {
	args := m.Called()
	return args.Get(0).(*runtime.Scheme)
}

func (m *MockClient) Status() client.StatusWriter {
	args := m.Called()
	return args.Get(0).(client.StatusWriter)
}

func (m *MockClient) SubResource(subResource string) client.SubResourceClient {
	args := m.Called(subResource)
	return args.Get(0).(client.SubResourceClient)
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
	mockHive.On("List", mock.Anything, mock.MatchedBy(func(obj interface{}) bool {
		_, ok := obj.(*corev1.NamespaceList)
		return ok
	}), mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		nsList := args.Get(1).(*corev1.NamespaceList)
		nsList.Items = []corev1.Namespace{*testNamespace}
	})

	mockHive.On("List", mock.Anything, mock.MatchedBy(func(obj interface{}) bool {
		_, ok := obj.(*hivev1.MachinePoolList)
		return ok
	}), mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		mpList := args.Get(1).(*hivev1.MachinePoolList)
		mpList.Items = []hivev1.MachinePool{*testMachinePool}
	})

	mp, err := GetInfraMachinePool(context.Background(), mockHive, "test-cluster")

	assert.NoError(t, err)
	assert.NotNil(t, mp)
	assert.Equal(t, "infra", mp.Spec.Name)
	assert.Equal(t, "r5.xlarge", mp.Spec.Platform.AWS.InstanceType)
	mockHive.AssertExpectations(t)
}

func TestGetInfraMachinePoolNoNamespace(t *testing.T) {
	mockHive := &MockClient{}
	mockHive.On("List", mock.Anything, mock.MatchedBy(func(obj interface{}) bool {
		_, ok := obj.(*corev1.NamespaceList)
		return ok
	}), mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		nsList := args.Get(1).(*corev1.NamespaceList)
		nsList.Items = []corev1.Namespace{}
	})

	mp, err := GetInfraMachinePool(context.Background(), mockHive, "test-cluster")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected 1 namespace, found 0")
	assert.Nil(t, mp)
	mockHive.AssertExpectations(t)
}

func TestGetInfraMachinePoolNoInfraPool(t *testing.T) {
	testNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-namespace",
			Labels: map[string]string{"api.openshift.com/id": "test-cluster"},
		},
	}

	mockHive := &MockClient{}
	mockHive.On("List", mock.Anything, mock.MatchedBy(func(obj interface{}) bool {
		_, ok := obj.(*corev1.NamespaceList)
		return ok
	}), mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		nsList := args.Get(1).(*corev1.NamespaceList)
		nsList.Items = []corev1.Namespace{*testNamespace}
	})

	mockHive.On("List", mock.Anything, mock.MatchedBy(func(obj interface{}) bool {
		_, ok := obj.(*hivev1.MachinePoolList)
		return ok
	}), mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		mpList := args.Get(1).(*hivev1.MachinePoolList)
		mpList.Items = []hivev1.MachinePool{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster-worker", Namespace: "test-namespace"},
				Spec:       hivev1.MachinePoolSpec{Name: "worker"},
			},
		}
	})

	mp, err := GetInfraMachinePool(context.Background(), mockHive, "test-cluster")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "did not find the infra machinepool")
	assert.Nil(t, mp)
	mockHive.AssertExpectations(t)
}

func TestCloneMachinePool(t *testing.T) {
	originalMp := &hivev1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cluster-infra",
			Namespace:       "test-namespace",
			ResourceVersion: "12345",
			Generation:      3,
			UID:             "abc-123",
			Finalizers:      []string{"hive.openshift.io/machinepool"},
		},
		Spec: hivev1.MachinePoolSpec{
			Name:     "infra",
			Replicas: int64Ptr(2),
			Labels: map[string]string{
				"node-role.kubernetes.io/infra": "",
			},
			Platform: hivev1.MachinePoolPlatform{
				AWS: &hivev1aws.MachinePoolPlatform{
					InstanceType: "r5.xlarge",
					EC2RootVolume: hivev1aws.EC2RootVolume{
						IOPS: 3000,
						Size: 300,
						Type: "io1",
					},
				},
			},
		},
	}

	t.Run("success - changes volume type", func(t *testing.T) {
		result, err := CloneMachinePool(originalMp, func(mp *hivev1.MachinePool) error {
			mp.Spec.Platform.AWS.Type = "gp3"
			mp.Spec.Platform.AWS.IOPS = 0
			return nil
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Verify modifier was applied
		assert.Equal(t, "gp3", result.Spec.Platform.AWS.Type)
		assert.Equal(t, 0, result.Spec.Platform.AWS.IOPS)

		// Verify metadata was reset
		assert.Empty(t, result.ResourceVersion)
		assert.Equal(t, int64(0), result.Generation)
		assert.Empty(t, string(result.UID))
		assert.Empty(t, result.Finalizers)
		assert.Equal(t, metav1.Time{}, result.CreationTimestamp)

		// Verify other fields preserved
		assert.Equal(t, "test-cluster-infra", result.Name)
		assert.Equal(t, "test-namespace", result.Namespace)
		assert.Equal(t, "infra", result.Spec.Name)
		assert.Equal(t, int64(2), *result.Spec.Replicas)
		assert.Equal(t, "r5.xlarge", result.Spec.Platform.AWS.InstanceType)
		assert.Equal(t, 300, result.Spec.Platform.AWS.Size)

		// Verify original is unchanged
		assert.Equal(t, "io1", originalMp.Spec.Platform.AWS.Type)
		assert.Equal(t, 3000, originalMp.Spec.Platform.AWS.IOPS)
	})

	t.Run("nil modifier just clones", func(t *testing.T) {
		result, err := CloneMachinePool(originalMp, nil)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.ResourceVersion)
		assert.Equal(t, "io1", result.Spec.Platform.AWS.Type)
	})

	t.Run("modifier error is propagated", func(t *testing.T) {
		result, err := CloneMachinePool(originalMp, func(mp *hivev1.MachinePool) error {
			return fmt.Errorf("infra volumes are already gp3")
		})
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "infra volumes are already gp3")
	})
}

func TestNodesMatchExpectedCount(t *testing.T) {
	tests := []struct {
		name          string
		expectedCount int
		mockNodeList  *corev1.NodeList
		mockListError error
		expectedMatch bool
		expectedError error
	}{
		{
			name:          "matching count",
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
			expectedCount: 2,
			mockListError: fmt.Errorf("list error"),
			expectedError: fmt.Errorf("list error"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockClient := &MockClient{}
			mockClient.On("List", mock.Anything, mock.Anything, mock.Anything).
				Return(test.mockListError).
				Run(func(args mock.Arguments) {
					if test.mockNodeList != nil {
						arg := args.Get(1).(*corev1.NodeList)
						*arg = *test.mockNodeList
					}
				})

			match, err := nodesMatchExpectedCount(context.Background(), mockClient, labels.NewSelector(), test.expectedCount)

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

			mockClient.AssertExpectations(t)
		})
	}
}

func int64Ptr(i int64) *int64 {
	return &i
}
