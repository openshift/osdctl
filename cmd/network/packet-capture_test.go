package network

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPacketCaptureCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := []struct {
		title       string
		option      *packetCaptureOptions
		errExpected bool
	}{
		{
			title:       "succeed",
			option:      &packetCaptureOptions{},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			err := tc.option.complete(nil, nil)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}

func TestNewPacketCaptureOptions(t *testing.T) {
	streams := genericclioptions.IOStreams{}
	lazyClient := &k8s.LazyClient{}

	ops := newPacketCaptureOptions(streams, lazyClient)

	assert.NotNil(t, ops)
	assert.Equal(t, streams, ops.IOStreams)
	assert.Equal(t, lazyClient, ops.kubeCli)
}

func TestComplete(t *testing.T) {
	tests := []struct {
		name   string
		reason string
	}{
		{
			name:   "without_reason",
			reason: "",
		},
		{
			name:   "with_reason",
			reason: "OHSS-1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := &packetCaptureOptions{
				reason:  tt.reason,
				kubeCli: &k8s.LazyClient{},
			}
			err := ops.complete(nil, nil)
			if err != nil {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDesiredPacketCaptureDaemonSet(t *testing.T) {
	ops := &packetCaptureOptions{
		name:             "test-capture",
		namespace:        "default",
		nodeLabelKey:     "node-role.kubernetes.io/worker",
		nodeLabelValue:   "",
		duration:         60,
		captureInterface: "eth0",
	}

	key := types.NamespacedName{Name: ops.name, Namespace: ops.namespace}
	ds := desiredPacketCaptureDaemonSet(ops, key)

	assert.Equal(t, ops.name, ds.Name)
	assert.Equal(t, ops.namespace, ds.Namespace)
	assert.Equal(t, map[string]string{"app": ops.name}, ds.Spec.Selector.MatchLabels)
	assert.Equal(t, true, ds.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, 1, len(ds.Spec.Template.Spec.InitContainers))
	assert.Equal(t, 1, len(ds.Spec.Template.Spec.Containers))
}

func TestDesiredPacketCapturePod(t *testing.T) {
	ops := &packetCaptureOptions{
		name:             "test-capture",
		namespace:        "test-ns",
		nodeLabelKey:     "test-key",
		nodeLabelValue:   "test-value",
		duration:         60,
		captureInterface: "test-interface",
	}

	key := types.NamespacedName{Name: ops.name, Namespace: ops.namespace}
	pod := desiredPacketCapturePod(ops, key)

	assert.Equal(t, ops.name, pod.Name)
	assert.Equal(t, ops.namespace, pod.Namespace)
	assert.Equal(t, map[string]string{"app": ops.name}, pod.Labels)
	assert.Equal(t, map[string]string{ops.nodeLabelKey: ops.nodeLabelValue}, pod.Spec.NodeSelector)
	assert.True(t, pod.Spec.HostNetwork)
}

func TestDeletePacketCapturePod(t *testing.T) {
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-capture-pod",
			Namespace: "test-namespace",
		},
	}

	tests := []struct {
		name      string
		pod       *corev1.Pod
		mockError error
		wantError bool
	}{
		{
			name:      "successful_pod_deletion",
			pod:       testPod,
			mockError: nil,
			wantError: false,
		},
		{
			name:      "failed_pod_deletion",
			pod:       testPod,
			mockError: fmt.Errorf("deletion failed"),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockKubeClient{}
			mockClient.On("Delete", mock.Anything, tt.pod, mock.Anything).Return(tt.mockError)

			opts := &packetCaptureOptions{
				kubeCli: mockClient.ToLazyClient(),
			}

			err := deletePacketCapturePod(opts, tt.pod)

			if tt.wantError {
				assert.Error(t, err)
				expectedErr := fmt.Errorf("failed to delete Pod %s/%s: %v", tt.pod.Namespace, tt.pod.Name, tt.mockError)
				assert.Equal(t, expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeletePacketCaptureDaemonSet(t *testing.T) {
	testDaemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-capture-daemonset",
			Namespace: "test-namespace",
		},
	}

	tests := []struct {
		name      string
		ds        *appsv1.DaemonSet
		mockError error
		wantError bool
	}{
		{
			name:      "successful_daemonset_deletion",
			ds:        testDaemonSet,
			mockError: nil,
			wantError: false,
		},
		{
			name:      "failed_daemonset_deletion",
			ds:        testDaemonSet,
			mockError: fmt.Errorf("deletion failed"),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockKubeClient{}
			mockClient.On("Delete", mock.Anything, tt.ds, mock.Anything).Return(tt.mockError)

			opts := &packetCaptureOptions{
				kubeCli: mockClient.ToLazyClient(),
			}
			err := deletePacketCaptureDaemonSet(opts, tt.ds)
			if tt.wantError {
				assert.Error(t, err)
				expectedErr := fmt.Errorf("failed to delete daemonset %s/%s: %v", tt.ds.Namespace, tt.ds.Name, tt.mockError)
				assert.Equal(t, expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreatePacketCapturePod(t *testing.T) {
	tests := []struct {
		name      string
		pod       *corev1.Pod
		setupFunc func(client *fake.ClientBuilder)
		wantError bool
	}{
		{
			name: "successfully_create_pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
			},
			setupFunc: nil,
			wantError: false,
		},
		{
			name: "error_creating_pod_already_exists",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-pod",
					Namespace: "default",
				},
			},
			setupFunc: func(client *fake.ClientBuilder) {
				existingPod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-pod",
						Namespace: "default",
					},
				}
				client.WithObjects(existingPod)
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientBuilder := fake.NewClientBuilder()
			if tt.setupFunc != nil {
				tt.setupFunc(clientBuilder)
			}
			fakeClient := clientBuilder.Build()

			opts := &packetCaptureOptions{
				kubeCli: k8s.LazyClientInit(fakeClient),
			}
			err := createPacketCapturePod(opts, tt.pod)
			if tt.wantError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantError {
				createdPod := &corev1.Pod{}
				err = fakeClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      tt.pod.Name,
						Namespace: tt.pod.Namespace,
					},
					createdPod)
				if err != nil {
					t.Errorf("failed to get created pod: %v", err)
				}
				if createdPod.Name != tt.pod.Name {
					t.Errorf("created pod name = %s, want %s", createdPod.Name, tt.pod.Name)
				}
				if createdPod.Namespace != tt.pod.Namespace {
					t.Errorf("created pod namespace = %s, want %s", createdPod.Namespace, tt.pod.Namespace)
				}
			}
		})
	}
}

func TestCreatePacketCaptureDaemonSet(t *testing.T) {
	tests := []struct {
		name      string
		ds        *appsv1.DaemonSet
		setupFunc func(client *fake.ClientBuilder)
		wantError bool
	}{
		{
			name: "successfully_create_daemonset",
			ds: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-daemonset",
					Namespace: "default",
				},
			},
			setupFunc: nil,
			wantError: false,
		},
		{
			name: "error_creating_daemonset_already_exists",
			ds: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-daemonset",
					Namespace: "default",
				},
			},
			setupFunc: func(client *fake.ClientBuilder) {
				existingDaemonSet := &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-daemonset",
						Namespace: "default",
					},
				}
				client.WithObjects(existingDaemonSet)
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientBuilder := fake.NewClientBuilder()
			if tt.setupFunc != nil {
				tt.setupFunc(clientBuilder)
			}
			fakeClient := clientBuilder.Build()
			opts := &packetCaptureOptions{
				kubeCli: k8s.LazyClientInit(fakeClient),
			}
			err := createPacketCaptureDaemonSet(opts, tt.ds)
			if tt.wantError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantError {
				createdDaemonSet := &appsv1.DaemonSet{}
				err = fakeClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      tt.ds.Name,
						Namespace: tt.ds.Namespace,
					},
					createdDaemonSet)
				if err != nil {
					t.Errorf("failed to get created daemonset: %v", err)
				}
				if createdDaemonSet.Name != tt.ds.Name {
					t.Errorf("created daemonset name = %s, want %s", createdDaemonSet.Name, tt.ds.Name)
				}
				if createdDaemonSet.Namespace != tt.ds.Namespace {
					t.Errorf("created daemonset namespace = %s, want %s", createdDaemonSet.Namespace, tt.ds.Namespace)
				}
			}
		})
	}
}

func TestWaitForPacketCapturePod(t *testing.T) {
	tests := []struct {
		name      string
		pod       *corev1.Pod
		updatePod func(*corev1.Pod)
		wantError bool
	}{
		{
			name: "pod_becomes_running",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
			},
			updatePod: func(pod *corev1.Pod) {
				pod.Status.Phase = corev1.PodRunning
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().Build()
			opts := &packetCaptureOptions{
				kubeCli: k8s.LazyClientInit(fakeClient),
			}
			if tt.updatePod != nil {
				err := fakeClient.Create(context.TODO(), tt.pod)
				if err != nil {
					t.Fatalf("failed to create test pod: %v", err)
				}
				tt.updatePod(tt.pod)
				err = fakeClient.Status().Update(context.TODO(), tt.pod)
				if err != nil {
					t.Fatalf("failed to update pod status: %v", err)
				}
			}
			err := waitForPacketCapturePod(opts, tt.pod)
			if tt.wantError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.updatePod != nil {
				finalPod := &corev1.Pod{}
				err = fakeClient.Get(context.TODO(),
					types.NamespacedName{
						Name:      tt.pod.Name,
						Namespace: tt.pod.Namespace,
					},
					finalPod)
				if err != nil {
					t.Errorf("failed to get final pod state: %v", err)
				}

				if !tt.wantError && finalPod.Status.Phase != corev1.PodRunning {
					t.Errorf("expected pod to be running, got phase: %v", finalPod.Status.Phase)
				}
			}
		})
	}
}

func TestEnsurePacketCapturePod(t *testing.T) {
	tests := []struct {
		name       string
		objects    []runtime.Object
		opts       *packetCaptureOptions
		wantErr    bool
		errMessage string
	}{
		{
			name:    "successfully_create_pod",
			objects: []runtime.Object{},
			opts: &packetCaptureOptions{
				name:      "test-pod",
				namespace: "default",
			},
			wantErr: false,
		},
		{
			name: "pod_already_exists",
			objects: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
					},
				},
			},
			opts: &packetCaptureOptions{
				name:      "test-pod",
				namespace: "default",
			},
			wantErr:    true,
			errMessage: "test-pod Pod already exists in the default namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.objects...).Build()
			tt.opts.kubeCli = k8s.LazyClientInit(fakeClient)
			pod, err := ensurePacketCapturePod(tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if err.Error() != tt.errMessage {
					t.Errorf("expected error message %q, got %q", tt.errMessage, err.Error())
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if pod.Name != tt.opts.name {
				t.Errorf("expected pod name %s, got %s", tt.opts.name, pod.Name)
			}
			if pod.Namespace != tt.opts.namespace {
				t.Errorf("expected pod namespace %s, got %s", tt.opts.namespace, pod.Namespace)
			}
			var createdPod corev1.Pod
			err = fakeClient.Get(context.Background(), client.ObjectKey{
				Name:      tt.opts.name,
				Namespace: tt.opts.namespace,
			}, &createdPod)
			if err != nil {
				t.Errorf("pod was not created in client: %v", err)
			}
		})
	}
}

func TestWaitForPacketCaptureContainerRunning(t *testing.T) {
	tests := []struct {
		name    string
		pod     *corev1.Pod
		wantErr bool
		setupFn func(*fake.ClientBuilder)
	}{
		{
			name: "container_running",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-ns",
				},
			},
			wantErr: false,
			setupFn: func(b *fake.ClientBuilder) {
				b.WithObjects(&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-ns",
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{},
								},
							},
						},
					},
				})
			},
		},
		{
			name: "container_not_running",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-ns",
				},
			},
			wantErr: true,
			setupFn: func(b *fake.ClientBuilder) {
				b.WithObjects(&corev1.Pod{
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Waiting: &corev1.ContainerStateWaiting{},
								},
							},
						},
					},
				})
			},
		},
		{
			name: "no_container_statuses",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-ns",
				},
			},
			wantErr: true,
			setupFn: func(b *fake.ClientBuilder) {
				b.WithObjects(&corev1.Pod{
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{},
					},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder()
			if tt.setupFn != nil {
				tt.setupFn(builder)
			}
			fakeClient := builder.Build()

			o := &packetCaptureOptions{
				kubeCli: k8s.LazyClientInit(fakeClient),
			}

			err := waitForPacketCaptureContainerRunning(o, tt.pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("waitForPacketCaptureContainerRunning() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWaitForPacketCaptureDaemonset(t *testing.T) {
	tests := []struct {
		name        string
		daemonset   *appsv1.DaemonSet
		updateDS    func(*appsv1.DaemonSet)
		wantErr     bool
		errContains string
	}{
		{
			name: "daemonset_ready",
			daemonset: &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds",
					Namespace: "test-ns",
				},
			},
			updateDS: func(ds *appsv1.DaemonSet) {
				ds.Status.NumberReady = 1
				ds.Status.NumberAvailable = 1
				ds.Status.DesiredNumberScheduled = 1
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lazyClient := fake.NewClientBuilder().Build()
			opts := &packetCaptureOptions{
				kubeCli: k8s.LazyClientInit(lazyClient),
			}
			if tt.updateDS != nil {
				tt.updateDS(tt.daemonset)
			}
			err := lazyClient.Create(context.TODO(), tt.daemonset)
			if err != nil {
				t.Fatalf("failed to create test daemonset: %v", err)
			}

			err = waitForPacketCaptureDaemonset(opts, tt.daemonset)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error message %q does not contain %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCopyFilesFromPod(t *testing.T) {
	tests := []struct {
		name    string
		pod     *corev1.Pod
		opts    *packetCaptureOptions
		wantErr bool
		setup   func()
		cleanup func()
	}{
		{
			name: "mkdir_error",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: "test-node",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-ns",
				},
			},
			opts: &packetCaptureOptions{
				startTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			setup: func() {
				os.Create(outputDir)
			},
			cleanup: func() {
				os.Remove(outputDir)
			},
			wantErr: true,
		},
		{
			name: "oc_cp_command_error",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					NodeName: "nonexistent-node",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nonexistent-pod",
					Namespace: "nonexistent-ns",
				},
			},
			opts: &packetCaptureOptions{
				startTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			setup: func() {
				os.MkdirAll(outputDir, 0750)
			},
			cleanup: func() {
				os.RemoveAll(outputDir)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			defer func() {
				if tt.cleanup != nil {
					tt.cleanup()
				}
			}()

			err := copyFilesFromPod(tt.opts, tt.pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("copyFilesFromPod() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				expectedFileName := fmt.Sprintf("%s/%s-%s.pcap",
					outputDir,
					tt.pod.Spec.NodeName,
					tt.opts.startTime.UTC().Format("20060102T150405"))

				if _, err := os.Stat(expectedFileName); os.IsNotExist(err) {
					t.Errorf("Expected file %s was not created", expectedFileName)
				}
			}
		})
	}
}

func TestHasPacketCaptureDaemonSet(t *testing.T) {
	tests := []struct {
		name      string
		key       types.NamespacedName
		setupFunc func(client *fake.ClientBuilder)
		wantFound bool
		wantError bool
	}{
		{
			name: "daemonset_exists",
			key: types.NamespacedName{
				Name:      "test-daemonset",
				Namespace: "default",
			},
			setupFunc: func(client *fake.ClientBuilder) {
				existingDaemonSet := &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-daemonset",
						Namespace: "default",
					},
				}
				client.WithObjects(existingDaemonSet)
			},
			wantFound: true,
			wantError: false,
		},
		{
			name: "daemonset_not_found",
			key: types.NamespacedName{
				Name:      "nonexistent-daemonset",
				Namespace: "default",
			},
			setupFunc: func(client *fake.ClientBuilder) {
			},
			wantFound: false,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientBuilder := fake.NewClientBuilder()
			if tt.setupFunc != nil {
				tt.setupFunc(clientBuilder)
			}
			fakeClient := clientBuilder.Build()
			opts := &packetCaptureOptions{
				kubeCli: k8s.LazyClientInit(fakeClient),
			}
			found, err := hasPacketCaptureDaemonSet(opts, tt.key)

			if tt.wantError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if found != tt.wantFound {
				t.Errorf("expected found = %v, got %v", tt.wantFound, found)
			}
		})
	}
}

type MockKubeClient struct {
	mock.Mock
	client.Client
}

func (m *MockKubeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

// Assuming your LazyClient is structured like this:
type LazyClient struct {
	client client.Client
}

func (m *MockKubeClient) ToLazyClient() *k8s.LazyClient {
	return k8s.LazyClientMock(m)
}
