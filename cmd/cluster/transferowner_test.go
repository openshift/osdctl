package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	hiveinternalv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	workv1 "open-cluster-management.io/api/work/v1"
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

func (m *MockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
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

func buildTestManifestWork(t *testing.T, secretName string, pullSecretData []byte, hcPullSecretRef string) *workv1.ManifestWork {
	t.Helper()
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "clusters",
		},
		Data: map[string][]byte{
			".dockerconfigjson": pullSecretData,
		},
	}
	secretJSON, err := json.Marshal(secret)
	require.NoError(t, err)

	hc := &hypershiftv1beta1.HostedCluster{
		TypeMeta: metav1.TypeMeta{Kind: "HostedCluster", APIVersion: "hypershift.openshift.io/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "clusters",
		},
		Spec: hypershiftv1beta1.HostedClusterSpec{
			PullSecret: corev1.LocalObjectReference{Name: hcPullSecretRef},
		},
	}
	hcJSON, err := json.Marshal(hc)
	require.NoError(t, err)

	cm := map[string]interface{}{
		"kind":       "ConfigMap",
		"apiVersion": "v1",
		"metadata":   map[string]interface{}{"name": "unrelated-config", "namespace": "clusters"},
		"data":       map[string]interface{}{"key": "value"},
	}
	cmJSON, err := json.Marshal(cm)
	require.NoError(t, err)

	return &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cluster-id",
			Namespace:       "test-mgmt",
			ResourceVersion: "100",
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{RawExtension: runtime.RawExtension{Raw: cmJSON}},
					{RawExtension: runtime.RawExtension{Raw: secretJSON}},
					{RawExtension: runtime.RawExtension{Raw: hcJSON}},
				},
			},
		},
	}
}

func TestUpdateManifestWorkRetriesOnConflict(t *testing.T) {
	oldPullSecret := []byte(`{"auths":{"old.registry":{"auth":"old-token","email":"old@test.com"}}}`)
	newPullSecret := []byte(`{"auths":{"new.registry":{"auth":"new-token","email":"new@test.com"}}}`)

	secretNamePrefix := "mycluster-pull"
	oldSecretName := secretNamePrefix + "-abc123"
	newSecretName := secretNamePrefix + "-def456"

	originalCMRaw := func() []byte {
		mw := buildTestManifestWork(t, oldSecretName, oldPullSecret, oldSecretName)
		raw := make([]byte, len(mw.Spec.Workload.Manifests[0].Raw))
		copy(raw, mw.Spec.Workload.Manifests[0].Raw)
		return raw
	}()

	var getCalls atomic.Int32
	var updateCalls atomic.Int32

	conflictErr := apierrors.NewConflict(
		schema.GroupResource{Group: "work.open-cluster-management.io", Resource: "manifestworks"},
		"test-cluster-id",
		errors.New("the object has been modified"),
	)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		getCalls.Add(1)

		freshMW := buildTestManifestWork(t, oldSecretName, oldPullSecret, oldSecretName)

		if err := updateManifestWorkPayloads(freshMW, secretNamePrefix, newSecretName, newPullSecret); err != nil {
			return err
		}

		call := updateCalls.Add(1)
		if call == 1 {
			return conflictErr
		}

		// Verify final state on successful update
		assert.Equal(t, originalCMRaw, freshMW.Spec.Workload.Manifests[0].Raw,
			"unrelated manifest should not be modified")

		var updatedSecret corev1.Secret
		assert.NoError(t, json.Unmarshal(freshMW.Spec.Workload.Manifests[1].Raw, &updatedSecret))
		assert.Equal(t, newSecretName, updatedSecret.Name)
		var mergedAuths map[string]interface{}
		assert.NoError(t, json.Unmarshal(updatedSecret.Data[".dockerconfigjson"], &mergedAuths))
		auths := mergedAuths["auths"].(map[string]interface{})
		assert.Contains(t, auths, "old.registry", "old registry auth should be preserved")
		assert.Contains(t, auths, "new.registry", "new registry auth should be added")

		var updatedHC hypershiftv1beta1.HostedCluster
		assert.NoError(t, json.Unmarshal(freshMW.Spec.Workload.Manifests[2].Raw, &updatedHC))
		assert.Equal(t, newSecretName, updatedHC.Spec.PullSecret.Name)

		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, int32(2), getCalls.Load(), "should GET twice (initial + retry after conflict)")
	assert.Equal(t, int32(2), updateCalls.Load(), "should UPDATE twice (conflict + success)")
}

func TestUpdateManifestWorkPreservesUnrelatedManifests(t *testing.T) {
	oldPullSecret := []byte(`{"auths":{"registry.example.com":{"auth":"token","email":"e@e.com"}}}`)
	newPullSecret := []byte(`{"auths":{"new.example.com":{"auth":"new-token","email":"n@n.com"}}}`)

	secretNamePrefix := "mycluster-pull"
	oldSecretName := secretNamePrefix + "-aaa111"
	newSecretName := secretNamePrefix + "-bbb222"

	mw := buildTestManifestWork(t, oldSecretName, oldPullSecret, oldSecretName)

	originalRaws := make([][]byte, len(mw.Spec.Workload.Manifests))
	for i, m := range mw.Spec.Workload.Manifests {
		originalRaws[i] = make([]byte, len(m.Raw))
		copy(originalRaws[i], m.Raw)
	}

	err := updateManifestWorkPayloads(mw, secretNamePrefix, newSecretName, newPullSecret)
	assert.NoError(t, err)

	assert.Equal(t, originalRaws[0], mw.Spec.Workload.Manifests[0].Raw,
		"ConfigMap manifest bytes should be untouched")
	assert.NotEqual(t, originalRaws[1], mw.Spec.Workload.Manifests[1].Raw,
		"Secret manifest should be modified")
	assert.NotEqual(t, originalRaws[2], mw.Spec.Workload.Manifests[2].Raw,
		"HostedCluster manifest should be modified")
	assert.Equal(t, len(originalRaws), len(mw.Spec.Workload.Manifests),
		"manifest count should not change")
}

func TestUpdateManifestWorkIndexShiftSafety(t *testing.T) {
	oldPullSecret := []byte(`{"auths":{"registry.example.com":{"auth":"token","email":"e@e.com"}}}`)
	newPullSecret := []byte(`{"auths":{"new.example.com":{"auth":"new-token","email":"n@n.com"}}}`)

	secretNamePrefix := "mycluster-pull"
	oldSecretName := secretNamePrefix + "-aaa111"
	newSecretName := secretNamePrefix + "-bbb222"

	var updateCalls atomic.Int32

	// Simulates a scenario where the ManifestWork gains a new manifest between retries.
	// On first attempt: [ConfigMap, Secret, HostedCluster]
	// On retry (after conflict): [NewNamespace, ConfigMap, Secret, HostedCluster]
	// Content-based lookup (by Kind) finds resources regardless of position.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		call := updateCalls.Add(1)

		mw := buildTestManifestWork(t, oldSecretName, oldPullSecret, oldSecretName)

		if call > 1 {
			// Another controller prepended a Namespace manifest, shifting all indices
			ns := map[string]interface{}{
				"kind": "Namespace", "apiVersion": "v1",
				"metadata": map[string]interface{}{"name": "new-namespace"},
			}
			nsJSON, err := json.Marshal(ns)
			require.NoError(t, err)
			shifted := make([]workv1.Manifest, 0, len(mw.Spec.Workload.Manifests)+1)
			shifted = append(shifted, workv1.Manifest{RawExtension: runtime.RawExtension{Raw: nsJSON}})
			shifted = append(shifted, mw.Spec.Workload.Manifests...)
			mw.Spec.Workload.Manifests = shifted
		}

		if err := updateManifestWorkPayloads(mw, secretNamePrefix, newSecretName, newPullSecret); err != nil {
			return err
		}

		if call == 1 {
			return apierrors.NewConflict(
				schema.GroupResource{Group: "work.open-cluster-management.io", Resource: "manifestworks"},
				"test-cluster-id",
				errors.New("the object has been modified"),
			)
		}

		// On retry with shifted array: verify correct resources were updated
		for _, manifest := range mw.Spec.Workload.Manifests {
			var meta struct {
				Kind string `json:"kind"`
			}
			require.NoError(t, json.Unmarshal(manifest.Raw, &meta))

			switch meta.Kind {
			case "Secret":
				var s corev1.Secret
				assert.NoError(t, json.Unmarshal(manifest.Raw, &s))
				assert.Equal(t, newSecretName, s.Name, "Secret should have new name despite index shift")
			case "HostedCluster":
				var hc hypershiftv1beta1.HostedCluster
				assert.NoError(t, json.Unmarshal(manifest.Raw, &hc))
				assert.Equal(t, newSecretName, hc.Spec.PullSecret.Name,
					"HostedCluster should reference new secret despite index shift")
			}
		}

		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, int32(2), updateCalls.Load())
}
