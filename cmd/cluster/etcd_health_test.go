package cluster

import (
	"context"
	"fmt"
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func (m *MockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	args := m.Called(ctx, list, opts)
	return args.Error(0)
}

func TestControlplaneNodeStatus(t *testing.T) {

	mockClient := new(MockClient)

	tests := []struct {
		name            string
		configureClient func() client.Client
		expectedError   error
		expectedOutput  string
	}{
		{
			name: "Test with nodes and valid conditions",
			configureClient: func() client.Client {

				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				}

				k8sClient := fake.NewClientBuilder().WithObjects(node).Build()
				return k8sClient
			},
			expectedError:  nil,
			expectedOutput: "+----------------------------------------------------------------+\n|                CONTROLPLANE NODE STATUS                        |\n+----------------------------------------------------------------+\nnode1\tReady\tTrue\n",
		},
		{
			name: "Test with no nodes found",
			configureClient: func() client.Client {

				k8sClient := fake.NewClientBuilder().Build()
				return k8sClient
			},
			expectedError:  nil,
			expectedOutput: "+----------------------------------------------------------------+\n|                CONTROLPLANE NODE STATUS                        |\n+----------------------------------------------------------------+\n",
		},
		{
			name: "Test with List failure",
			configureClient: func() client.Client {

				mockClient.On("List", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("list error"))
				return mockClient
			},
			expectedError:  fmt.Errorf("list error"),
			expectedOutput: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			kubeCli := tt.configureClient()

			err := ControlplaneNodeStatus(kubeCli)

			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEtcdPodStatus(t *testing.T) {
	tests := []struct {
		name          string
		mockClient    func() client.Client
		expectedPods  *corev1.PodList
		expectedError error
	}{
		{
			name: "Test with pods having ready containers",
			mockClient: func() client.Client {

				pod1 := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "etcd-pod-1",
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Ready: true,
							},
							{
								Ready: false,
							},
						},
					},
				}

				pod2 := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "etcd-pod-2",
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Ready: true,
							},
						},
					},
				}

				client := new(MockClient)
				client.On("List", mock.Anything, mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {

					list := args.Get(1).(*corev1.PodList)
					list.Items = append(list.Items, *pod1, *pod2)
				})

				return client
			},
			expectedPods: &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "etcd-pod-1",
						},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Ready: true,
								},
								{
									Ready: false,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "etcd-pod-2",
						},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Ready: true,
								},
							},
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "Test with no pods found",
			mockClient: func() client.Client {

				return fake.NewClientBuilder().Build()
			},
			expectedPods: &corev1.PodList{
				Items: []corev1.Pod{},
			},
			expectedError: nil,
		},
		{
			name: "Test with List failure",
			mockClient: func() client.Client {

				client := new(MockClient)
				client.On("List", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("list error"))
				return client
			},
			expectedPods:  nil,
			expectedError: fmt.Errorf("list error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			kubeCli := tt.mockClient()

			pods, err := EtcdPodStatus(kubeCli)

			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

			if tt.expectedPods != nil {
				assert.Equal(t, tt.expectedPods, pods)
			} else {
				assert.Nil(t, pods)
			}
		})
	}
}

func TestEtcdCrStatus(t *testing.T) {
	tests := []struct {
		name          string
		mockClient    func() client.Client
		expectedName  string
		expectedError error
	}{
		{
			name: "Test with valid Etcd CR status",
			mockClient: func() client.Client {

				mockClient := new(MockClient)

				etcdCR := &operatorv1.Etcd{
					Status: operatorv1.EtcdStatus{
						StaticPodOperatorStatus: operatorv1.StaticPodOperatorStatus{
							OperatorStatus: operatorv1.OperatorStatus{

								Conditions: []operatorv1.OperatorCondition{
									{
										Type:    "EtcdMembersAvailable",
										Message: "etcd-member-1 is unavailable, etcd-member-2 is available",
									},
								},
							},
						},
					},
				}

				mockClient.On("Get", mock.Anything, client.ObjectKey{Name: "cluster"}, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					arg := args.Get(2).(*operatorv1.Etcd)
					*arg = *etcdCR
				}).Return(nil)

				return mockClient
			},
			expectedName:  "etcd-member-2",
			expectedError: nil,
		},
		{
			name: "Test with no conditions in Etcd CR",
			mockClient: func() client.Client {

				mockClient := new(MockClient)

				etcdCR := &operatorv1.Etcd{
					Status: operatorv1.EtcdStatus{
						StaticPodOperatorStatus: operatorv1.StaticPodOperatorStatus{
							OperatorStatus: operatorv1.OperatorStatus{

								Conditions: []operatorv1.OperatorCondition{},
							},
						},
					},
				}

				mockClient.On("Get", mock.Anything, client.ObjectKey{Name: "cluster"}, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					arg := args.Get(2).(*operatorv1.Etcd)
					*arg = *etcdCR
				}).Return(nil)

				return mockClient
			},
			expectedName:  "",
			expectedError: nil,
		},
		{
			name: "Test with Get method error",
			mockClient: func() client.Client {

				mockClient := new(MockClient)

				mockClient.On("Get", mock.Anything, client.ObjectKey{Name: "cluster"}, mock.Anything, mock.Anything).Return(fmt.Errorf("failed to get Etcd CR"))

				return mockClient
			},
			expectedName:  "",
			expectedError: fmt.Errorf("failed to get Etcd CR"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			kubeCli := tt.mockClient()

			name, err := EtcdCrStatus(kubeCli)

			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedName, name)
		})
	}
}
