package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	hiveinternalv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestValidateOldOwner(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := []struct {
		title             string
		subscription      *amv1.SubscriptionBuilder
		oldOwner          *amv1.AccountBuilder
		oldOrganizationId string
		creator           *amv1.AccountBuilder
		okExpected        bool
	}{
		{
			title:             "valid old owner",
			oldOwner:          amv1.NewAccount().ID("test"),
			oldOrganizationId: "test",
			creator:           amv1.NewAccount().ID("test"),
			subscription:      amv1.NewSubscription().OrganizationID("test"),
			okExpected:        true,
		},
		{
			title:             "old-organization-id differs on subscription",
			oldOwner:          amv1.NewAccount().ID("test"),
			oldOrganizationId: "123",
			creator:           amv1.NewAccount().ID("test"),
			subscription:      amv1.NewSubscription().OrganizationID("test"),
			okExpected:        false,
		},
		{
			title:             "old owner differs on subscription",
			oldOwner:          amv1.NewAccount().ID("test"),
			oldOrganizationId: "123",
			creator:           amv1.NewAccount().ID("123"),
			subscription:      amv1.NewSubscription().OrganizationID("test"),
			okExpected:        false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			acc, _ := tc.oldOwner.Build()
			sub, _ := tc.subscription.Creator(tc.creator).Build()
			ok := validateOldOwner(tc.oldOrganizationId, sub, acc)
			if tc.okExpected {
				g.Expect(ok).Should(BeTrue())
			} else {
				g.Expect(ok).ShouldNot(BeTrue())
			}
		})
	}
}

// MockClient struct to mock the necessary client methods (Create, Get, Delete)
type MockClient struct {
	mock.Mock
	client.Client
}

func (m *MockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	args := m.Called(ctx, obj)
	return args.Error(0)
}

func (m *MockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	args := m.Called(ctx, key, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	args := m.Called(ctx, obj)
	return args.Error(0)
}

func (m *MockClient) Scheme() *runtime.Scheme {
	return runtime.NewScheme()
}

func (m *MockClient) Status() client.StatusWriter {
	return nil
}

func TestAwaitPullSecretSyncSet(t *testing.T) {
	hiveNamespace := "test-namespace"
	cdName := "test-cluster-deployment"

	tests := []struct {
		name               string
		mockCreateErr      error
		mockGetErr         error
		mockGetClusterSync *hiveinternalv1alpha1.ClusterSync
		mockDeleteErr      error
		expectedErr        string
		expectedSuccess    bool
	}{
		{
			name:          "Success",
			mockCreateErr: nil,
			mockGetErr:    nil,
			mockGetClusterSync: &hiveinternalv1alpha1.ClusterSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cdName,
					Namespace: hiveNamespace,
				},
				Status: hiveinternalv1alpha1.ClusterSyncStatus{
					SyncSets: []hiveinternalv1alpha1.SyncStatus{
						{
							Name:             "pull-secret-replacement",
							FirstSuccessTime: &metav1.Time{Time: time.Now()},
						},
					},
				},
			},
			mockDeleteErr:   nil,
			expectedErr:     "",
			expectedSuccess: true,
		},
		{
			name:               "CreateSyncSetFailure",
			mockCreateErr:      errors.New("failed to create"),
			mockGetErr:         nil,
			mockGetClusterSync: nil,
			mockDeleteErr:      nil,
			expectedErr:        "failed to create SyncSet",
			expectedSuccess:    false,
		},

		{
			name:               "GetClusterSyncFailure",
			mockCreateErr:      nil,
			mockGetErr:         errors.New("failed to get"),
			mockGetClusterSync: nil,
			mockDeleteErr:      nil,
			expectedErr:        "failed to get status for resource",
			expectedSuccess:    false,
		},
		{
			name:          "DeleteSyncSetFailure",
			mockCreateErr: nil,
			mockGetErr:    nil,
			mockGetClusterSync: &hiveinternalv1alpha1.ClusterSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cdName,
					Namespace: hiveNamespace,
				},
				Status: hiveinternalv1alpha1.ClusterSyncStatus{
					SyncSets: []hiveinternalv1alpha1.SyncStatus{
						{
							Name:             "pull-secret-replacement",
							FirstSuccessTime: &metav1.Time{Time: time.Now()},
						},
					},
				},
			},
			mockDeleteErr:   errors.New("failed to delete"),
			expectedErr:     "failed to delete SyncSet",
			expectedSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mockClient := new(MockClient)

			mockClient.On("Create", mock.Anything, mock.Anything).Return(tt.mockCreateErr)
			if tt.mockGetClusterSync != nil {
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						clusterSync := args.Get(2).(*hiveinternalv1alpha1.ClusterSync)
						*clusterSync = *tt.mockGetClusterSync
					}).
					Return(tt.mockGetErr)
			} else {
				mockClient.On("Get", mock.Anything, mock.AnythingOfType("types.NamespacedName"), mock.Anything, mock.Anything).
					Return(tt.mockGetErr)
			}
			mockClient.On("Delete", mock.Anything, mock.Anything).Return(tt.mockDeleteErr)

			err := awaitPullSecretSyncSet(hiveNamespace, cdName, mockClient)

			if tt.expectedSuccess {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
			}

		})
	}
}

func TestBuildNewSecret(t *testing.T) {
	tests := []struct {
		name           string
		oldPullSecret  []byte
		newPullSecret  []byte
		expectedResult []byte
		expectedError  string
	}{
		{
			name: "valid merge",
			oldPullSecret: []byte(`{
				"auths": {
					"old_registry": {
						"auth": "old_auth_token",
						"email": "old_email@example.com"
					}
				}
			}`),
			newPullSecret: []byte(`{
				"auths": {
					"new_registry": {
						"auth": "new_auth_token",
						"email": "new_email@example.com"
					}
				}
			}`),
			expectedResult: []byte(`{
				"auths": {
					"old_registry": {
						"auth": "old_auth_token",
						"email": "old_email@example.com"
					},
					"new_registry": {
						"auth": "new_auth_token",
						"email": "new_email@example.com"
					}
				}
			}`),
			expectedError: "",
		},
		{
			name: "invalid old pull secret",
			oldPullSecret: []byte(`{
				"auths": {
					"old_registry": {
						"auth": "old_auth_token",
						"email": "old_email@example.com"
					}
				}
			}`),
			newPullSecret: []byte(`{
				"auths": {
					"new_registry": {
						"auth": "new_auth_token",
						"email": "new_email@example.com"
					}
				}
			}`),
			expectedResult: nil,
			expectedError:  "invalid character 'a' looking for beginning of value",
		},
		{
			name: "empty new pull secret",
			oldPullSecret: []byte(`{
				"auths": {
					"old_registry": {
						"auth": "old_auth_token",
						"email": "old_email@example.com"
					}
				}
			}`),
			newPullSecret: []byte(`{
				"auths": {}
			}`),
			expectedResult: []byte(`{
				"auths": {
					"old_registry": {
						"auth": "old_auth_token",
						"email": "old_email@example.com"
					}
				}
			}`),
			expectedError: "",
		},
		{
			name: "empty old pull secret",
			oldPullSecret: []byte(`{
				"auths": {}
			}`),
			newPullSecret: []byte(`{
				"auths": {
					"new_registry": {
						"auth": "new_auth_token",
						"email": "new_email@example.com"
					}
				}
			}`),
			expectedResult: []byte(`{
				"auths": {
					"new_registry": {
						"auth": "new_auth_token",
						"email": "new_email@example.com"
					}
				}
			}`),
			expectedError: "",
		},
		{
			name: "invalid new pull secret",
			oldPullSecret: []byte(`{
				"auths": {
					"old_registry": {
						"auth": "old_auth_token",
						"email": "old_email@example.com"
					}
				}
			}`),
			newPullSecret: []byte(`{
				"auths": {`), // Invalid JSON
			expectedResult: nil,
			expectedError:  "unexpected end of JSON input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildNewSecret(tt.oldPullSecret, tt.newPullSecret)

			if err != nil && err.Error() != tt.expectedError {
				t.Errorf("expected error %v, got %v", tt.expectedError, err)
			}

			if tt.expectedResult != nil && !reflect.DeepEqual(result, tt.expectedResult) {
				var expectedMap, resultMap map[string]interface{}
				if err := json.Unmarshal(tt.expectedResult, &expectedMap); err != nil {
					t.Fatalf("error unmarshaling expected result: %v", err)
				}
				if err := json.Unmarshal(result, &resultMap); err != nil {
					t.Fatalf("error unmarshaling result: %v", err)
				}

				if !reflect.DeepEqual(expectedMap, resultMap) {
					t.Errorf("expected result %v, got %v", expectedMap, resultMap)
				}
			}
		})
	}
}
