package cluster

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Mocking Kubernetes client to test patchEtcd function
type MockKubeClient struct {
	client.Client
	mock.Mock
}

func (m *MockKubeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	args := m.Called(ctx, key, obj, opts)
	return args.Error(0)
}

func (m *MockKubeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	args := m.Called(ctx, obj, patch, opts)
	return args.Error(0)
}

func TestPatchEtcd(t *testing.T) {
	tests := []struct {
		title          string
		mockGetError   error
		mockPatchError error
		expectedError  error
		patch          string
	}{
		{
			title:          "successful patch",
			mockGetError:   nil,
			mockPatchError: nil,
			expectedError:  nil,
			patch:          `{"spec": {"replicas": 3}}`,
		},
		{
			title:          "etcd CR not found (Get failure)",
			mockGetError:   errors.New("resource not found"),
			mockPatchError: nil,
			expectedError:  errors.New("resource not found"),
			patch:          `{"spec": {"replicas": 3}}`,
		},
		{
			title:          "patch failure",
			mockGetError:   nil,
			mockPatchError: assert.AnError, // Simulating a patch failure
			expectedError:  assert.AnError,
			patch:          `{"spec": {"replicas": 3}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {

			mockClient := new(MockKubeClient)

			mockClient.On("Get", mock.Anything, client.ObjectKey{Name: "cluster"}, mock.Anything, mock.Anything).Return(tt.mockGetError)
			mockClient.On("Patch", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(tt.mockPatchError)

			err := patchEtcd(mockClient, tt.patch)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
			} else {
				assert.NoError(t, err)
			}

		})
	}
}
