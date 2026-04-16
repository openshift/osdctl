package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	ccov1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	hiveapiv1 "github.com/openshift/hive/apis/hive/v1"
	hiveinternalv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	mock_aws "github.com/openshift/osdctl/pkg/provider/aws/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, awsv1alpha1.AddToScheme(s))
	require.NoError(t, hiveapiv1.AddToScheme(s))
	require.NoError(t, hiveinternalv1alpha1.AddToScheme(s))
	require.NoError(t, ccov1.AddToScheme(s))
	return s
}

// testAccount returns a basic Account CR for testing.
func testAccount(byoc bool, stsMode bool) *awsv1alpha1.Account {
	return &awsv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-account",
			Namespace: awsAccountNamespace,
			Labels:    map[string]string{"iamUserId": "abcd"},
		},
		Spec: awsv1alpha1.AccountSpec{
			AwsAccountID:       "123456789012",
			BYOC:               byoc,
			ManualSTSMode:      stsMode,
			ClaimLinkNamespace: "uhc-production-test",
		},
	}
}

// testSecrets returns the two k8s secrets that RotateSecret updates.
func testSecrets() []runtime.Object {
	return []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-account-secret",
				Namespace: awsAccountNamespace,
			},
			Data: map[string][]byte{
				"aws_user_name":         []byte("old-user"),
				"aws_access_key_id":     []byte("old-key"),
				"aws_secret_access_key": []byte("old-secret"),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aws",
				Namespace: "uhc-production-test",
			},
			Data: map[string][]byte{
				"aws_user_name":         []byte("old-user"),
				"aws_access_key_id":     []byte("old-key"),
				"aws_secret_access_key": []byte("old-secret"),
			},
		},
	}
}

func testClusterDeployment() *hiveapiv1.ClusterDeployment {
	return &hiveapiv1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cd",
			Namespace: "uhc-production-test",
		},
	}
}

func testClusterSync(synced bool) *hiveinternalv1alpha1.ClusterSync {
	cs := &hiveinternalv1alpha1.ClusterSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cd",
			Namespace: "uhc-production-test",
		},
	}
	if synced {
		now := metav1.Now()
		cs.Status.SyncSets = []hiveinternalv1alpha1.SyncStatus{
			{
				Name:             "aws-sync",
				FirstSuccessTime: &now,
			},
		}
	}
	return cs
}

// testManagedClient creates a fake managed cluster client with optional
// CredentialRequest objects pre-seeded.
func testManagedClient(t *testing.T, crs ...runtime.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(crs...).Build()
}

func mockSimulateAllAllowed(mockClient *mock_aws.MockClient) *gomock.Call {
	return mockClient.EXPECT().
		SimulatePrincipalPolicy(gomock.Any()).
		Return(&iam.SimulatePrincipalPolicyOutput{
			EvaluationResults: []iamTypes.EvaluationResult{
				{EvalActionName: awsSdk.String("iam:CreateAccessKey"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
				{EvalActionName: awsSdk.String("iam:CreateUser"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
				{EvalActionName: awsSdk.String("iam:DeleteAccessKey"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
				{EvalActionName: awsSdk.String("iam:DeleteUser"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
				{EvalActionName: awsSdk.String("iam:DeleteUserPolicy"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
				{EvalActionName: awsSdk.String("iam:GetUser"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
				{EvalActionName: awsSdk.String("iam:GetUserPolicy"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
				{EvalActionName: awsSdk.String("iam:ListAccessKeys"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
				{EvalActionName: awsSdk.String("iam:PutUserPolicy"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
				{EvalActionName: awsSdk.String("iam:TagUser"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
			},
		}, nil)
}

func mockCreateAccessKey(mockClient *mock_aws.MockClient, username string) *gomock.Call {
	return mockClient.EXPECT().
		CreateAccessKey(gomock.Any()).
		DoAndReturn(func(input *iam.CreateAccessKeyInput) (*iam.CreateAccessKeyOutput, error) {
			return &iam.CreateAccessKeyOutput{
				AccessKey: &iamTypes.AccessKey{
					UserName:        input.UserName,
					AccessKeyId:     awsSdk.String("NEWKEY123"),
					SecretAccessKey: awsSdk.String("NEWSECRET456"),
				},
			}, nil
		})
}

func mockListAccessKeys(mockClient *mock_aws.MockClient, oldKeyID, newKeyID string) *gomock.Call {
	return mockClient.EXPECT().
		ListAccessKeys(gomock.Any()).
		Return(&iam.ListAccessKeysOutput{
			AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
				{AccessKeyId: awsSdk.String(oldKeyID)},
				{AccessKeyId: awsSdk.String(newKeyID)},
			},
		}, nil)
}

func TestRotateSecret_STSAccount(t *testing.T) {
	account := testAccount(false, true)
	out := &bytes.Buffer{}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	kubeCli := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()

	err := RotateSecret(context.Background(), &RotateSecretInput{
		AccountCRName:        "test-account",
		Account:              account,
		AwsClient:            mockClient,
		HiveKubeClient:       kubeCli,
		ManagedClusterClient: testManagedClient(t),
		Out:                  out,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "STS - No IAM User Credentials to Rotate")
}

func TestRotateSecret_MissingIamUserIdLabel(t *testing.T) {
	account := testAccount(false, false)
	delete(account.Labels, "iamUserId")

	out := &bytes.Buffer{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	kubeCli := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()

	err := RotateSecret(context.Background(), &RotateSecretInput{
		AccountCRName:        "test-account",
		Account:              account,
		AwsClient:            mockClient,
		HiveKubeClient:       kubeCli,
		ManagedClusterClient: testManagedClient(t),
		Out:                  out,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no label on Account CR for IAM User")
}

func TestRotateSecret_SuccessfulRotation(t *testing.T) {
	// Set fast polling for tests
	origInterval := SyncPollInterval
	origRetries := SyncMaxRetries
	SyncPollInterval = 0
	SyncMaxRetries = 1
	defer func() {
		SyncPollInterval = origInterval
		SyncMaxRetries = origRetries
	}()

	account := testAccount(false, false)
	secrets := testSecrets()
	cd := testClusterDeployment()
	cs := testClusterSync(true)

	objs := append(secrets, account, cd, cs)
	scheme := testScheme(t)
	kubeCli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithStatusSubresource(cs).Build()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	mockSimulateAllAllowed(mockClient)
	mockCreateAccessKey(mockClient, "osdManagedAdmin-abcd")
	mockListAccessKeys(mockClient, "OLDKEY999", "NEWKEY123")

	out := &bytes.Buffer{}

	err := RotateSecret(context.Background(), &RotateSecretInput{
		AccountCRName:        "test-account",
		Account:              account,
		AwsClient:            mockClient,
		HiveKubeClient:       kubeCli,
		ManagedClusterClient: testManagedClient(t),
		Out:                  out,
	})

	assert.NoError(t, err)
	assert.Contains(t, out.String(), "AWS creds updated on hive.")
	assert.Contains(t, out.String(), "Successfully rotated secrets for osdManagedAdmin-abcd")
	assert.Contains(t, out.String(), "OLDKEY999 (old - should be removed)")
	assert.Contains(t, out.String(), "NEWKEY123 (new - just created)")
	assert.Contains(t, out.String(), "rh-aws-saml-login")
}

func TestRotateSecret_SuccessfulRotationWithCCS(t *testing.T) {
	origInterval := SyncPollInterval
	origRetries := SyncMaxRetries
	SyncPollInterval = 0
	SyncMaxRetries = 1
	defer func() {
		SyncPollInterval = origInterval
		SyncMaxRetries = origRetries
	}()

	account := testAccount(true, false)
	secrets := testSecrets()
	byocSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "byoc",
			Namespace: "uhc-production-test",
		},
		Data: map[string][]byte{
			"aws_user_name":         []byte("old-ccs-user"),
			"aws_access_key_id":     []byte("old-ccs-key"),
			"aws_secret_access_key": []byte("old-ccs-secret"),
		},
	}
	cd := testClusterDeployment()
	cs := testClusterSync(true)

	objs := append(secrets, account, cd, cs, byocSecret)
	scheme := testScheme(t)
	kubeCli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithStatusSubresource(cs).Build()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	mockSimulateAllAllowed(mockClient)
	// First call: osdManagedAdmin key
	mockCreateAccessKey(mockClient, "osdManagedAdmin-abcd")
	mockListAccessKeys(mockClient, "OLDKEY999", "NEWKEY123")
	// Second call: osdCcsAdmin key
	mockCreateAccessKey(mockClient, "osdCcsAdmin")
	mockListAccessKeys(mockClient, "OLDCCSKEY", "NEWKEY123")

	out := &bytes.Buffer{}

	err := RotateSecret(context.Background(), &RotateSecretInput{
		AccountCRName:        "test-account",
		Account:              account,
		UpdateCcsCreds:       true,
		AwsClient:            mockClient,
		HiveKubeClient:       kubeCli,
		ManagedClusterClient: testManagedClient(t),
		Out:                  out,
	})

	assert.NoError(t, err)
	assert.Contains(t, out.String(), "Successfully rotated secrets for osdManagedAdmin-abcd")
	assert.Contains(t, out.String(), "Successfully rotated secrets for osdCcsAdmin")
}

func TestRotateSecret_CCSFlagOnNonBYOCAccount(t *testing.T) {
	origInterval := SyncPollInterval
	origRetries := SyncMaxRetries
	SyncPollInterval = 0
	SyncMaxRetries = 1
	defer func() {
		SyncPollInterval = origInterval
		SyncMaxRetries = origRetries
	}()

	account := testAccount(false, false)
	secrets := testSecrets()
	cd := testClusterDeployment()
	cs := testClusterSync(true)

	objs := append(secrets, account, cd, cs)
	scheme := testScheme(t)
	kubeCli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithStatusSubresource(cs).Build()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	mockSimulateAllAllowed(mockClient)
	mockCreateAccessKey(mockClient, "osdManagedAdmin-abcd")
	mockListAccessKeys(mockClient, "OLDKEY999", "NEWKEY123")

	out := &bytes.Buffer{}

	err := RotateSecret(context.Background(), &RotateSecretInput{
		AccountCRName:        "test-account",
		Account:              account,
		UpdateCcsCreds:       true,
		AwsClient:            mockClient,
		HiveKubeClient:       kubeCli,
		ManagedClusterClient: testManagedClient(t),
		Out:                  out,
	})

	assert.NoError(t, err)
	assert.Contains(t, out.String(), "Account is not CCS, skipping osdCcsAdmin credential rotation")
}

func TestRotateSecret_AdminUsernameFallback(t *testing.T) {
	origInterval := SyncPollInterval
	origRetries := SyncMaxRetries
	SyncPollInterval = 0
	SyncMaxRetries = 1
	defer func() {
		SyncPollInterval = origInterval
		SyncMaxRetries = origRetries
	}()

	account := testAccount(false, false)
	secrets := testSecrets()
	cd := testClusterDeployment()
	cs := testClusterSync(true)

	objs := append(secrets, account, cd, cs)
	scheme := testScheme(t)
	kubeCli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithStatusSubresource(cs).Build()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	// First simulate call fails (suffixed username)
	mockClient.EXPECT().
		SimulatePrincipalPolicy(gomock.Any()).
		DoAndReturn(func(input *iam.SimulatePrincipalPolicyInput) (*iam.SimulatePrincipalPolicyOutput, error) {
			if *input.PolicySourceArn == "arn:aws:iam::123456789012:user/osdManagedAdmin-abcd" {
				return &iam.SimulatePrincipalPolicyOutput{
					EvaluationResults: []iamTypes.EvaluationResult{
						{EvalActionName: awsSdk.String("iam:CreateAccessKey"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeExplicitDeny},
					},
				}, nil
			}
			// Second call with unsuffixed username succeeds
			return &iam.SimulatePrincipalPolicyOutput{
				EvaluationResults: []iamTypes.EvaluationResult{
					{EvalActionName: awsSdk.String("iam:CreateAccessKey"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:CreateUser"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:DeleteAccessKey"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:DeleteUser"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:DeleteUserPolicy"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:GetUser"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:GetUserPolicy"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:ListAccessKeys"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:PutUserPolicy"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:TagUser"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
				},
			}, nil
		}).Times(2)

	mockCreateAccessKey(mockClient, "osdManagedAdmin")
	mockListAccessKeys(mockClient, "OLDKEY999", "NEWKEY123")

	out := &bytes.Buffer{}

	err := RotateSecret(context.Background(), &RotateSecretInput{
		AccountCRName:        "test-account",
		Account:              account,
		AwsClient:            mockClient,
		HiveKubeClient:       kubeCli,
		ManagedClusterClient: testManagedClient(t),
		Out:                  out,
	})

	assert.NoError(t, err)
	assert.Contains(t, out.String(), "Permission verification failed for osdManagedAdmin-abcd, trying osdManagedAdmin...")
	assert.Contains(t, out.String(), "Successfully rotated secrets for osdManagedAdmin")
}

func TestRotateSecret_CreateAccessKeyNoSuchEntityFallback(t *testing.T) {
	origInterval := SyncPollInterval
	origRetries := SyncMaxRetries
	SyncPollInterval = 0
	SyncMaxRetries = 1
	defer func() {
		SyncPollInterval = origInterval
		SyncMaxRetries = origRetries
	}()

	account := testAccount(false, false)
	secrets := testSecrets()
	cd := testClusterDeployment()
	cs := testClusterSync(true)

	objs := append(secrets, account, cd, cs)
	scheme := testScheme(t)
	kubeCli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithStatusSubresource(cs).Build()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	mockSimulateAllAllowed(mockClient)

	// First CreateAccessKey call fails with NoSuchEntityException
	nse := &iamTypes.NoSuchEntityException{Message: awsSdk.String("user not found")}
	gomock.InOrder(
		mockClient.EXPECT().
			CreateAccessKey(gomock.Any()).
			Return(nil, nse),
		mockClient.EXPECT().
			CreateAccessKey(gomock.Any()).
			DoAndReturn(func(input *iam.CreateAccessKeyInput) (*iam.CreateAccessKeyOutput, error) {
				assert.Equal(t, "osdManagedAdmin", *input.UserName)
				return &iam.CreateAccessKeyOutput{
					AccessKey: &iamTypes.AccessKey{
						UserName:        input.UserName,
						AccessKeyId:     awsSdk.String("NEWKEY123"),
						SecretAccessKey: awsSdk.String("NEWSECRET456"),
					},
				}, nil
			}),
	)
	mockListAccessKeys(mockClient, "OLDKEY999", "NEWKEY123")

	out := &bytes.Buffer{}

	err := RotateSecret(context.Background(), &RotateSecretInput{
		AccountCRName:        "test-account",
		Account:              account,
		AwsClient:            mockClient,
		HiveKubeClient:       kubeCli,
		ManagedClusterClient: testManagedClient(t),
		Out:                  out,
	})

	assert.NoError(t, err)
	assert.Contains(t, out.String(), "Successfully rotated secrets for osdManagedAdmin")
}

func TestRotateSecret_SyncSetTimeout(t *testing.T) {
	origInterval := SyncPollInterval
	origRetries := SyncMaxRetries
	SyncPollInterval = 0
	SyncMaxRetries = 1
	defer func() {
		SyncPollInterval = origInterval
		SyncMaxRetries = origRetries
	}()

	account := testAccount(false, false)
	secrets := testSecrets()
	cd := testClusterDeployment()
	// ClusterSync without successful sync status
	cs := testClusterSync(false)

	objs := append(secrets, account, cd, cs)
	scheme := testScheme(t)
	kubeCli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithStatusSubresource(cs).Build()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	mockSimulateAllAllowed(mockClient)
	mockCreateAccessKey(mockClient, "osdManagedAdmin-abcd")
	mockListAccessKeys(mockClient, "OLDKEY999", "NEWKEY123")

	out := &bytes.Buffer{}

	err := RotateSecret(context.Background(), &RotateSecretInput{
		AccountCRName:        "test-account",
		Account:              account,
		AwsClient:            mockClient,
		HiveKubeClient:       kubeCli,
		ManagedClusterClient: testManagedClient(t),
		Out:                  out,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "syncset failed to sync")
}

func TestVerifyRotationPermissions(t *testing.T) {
	tests := []struct {
		name           string
		accountID      string
		username       string
		mockResponse   *iam.SimulatePrincipalPolicyOutput
		mockError      error
		expectedErr    bool
		expectedErrMsg string
	}{
		{
			name:      "all_permissions_allowed",
			accountID: "123456789012",
			username:  "osdManagedAdmin-test",
			mockResponse: &iam.SimulatePrincipalPolicyOutput{
				EvaluationResults: []iamTypes.EvaluationResult{
					{EvalActionName: awsSdk.String("iam:CreateAccessKey"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:CreateUser"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:DeleteAccessKey"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:DeleteUser"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:DeleteUserPolicy"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:GetUser"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:GetUserPolicy"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:ListAccessKeys"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:PutUserPolicy"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:TagUser"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
				},
			},
			expectedErr: false,
		},
		{
			name:      "some_permissions_denied",
			accountID: "123456789012",
			username:  "osdManagedAdmin-test",
			mockResponse: &iam.SimulatePrincipalPolicyOutput{
				EvaluationResults: []iamTypes.EvaluationResult{
					{EvalActionName: awsSdk.String("iam:CreateAccessKey"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
					{EvalActionName: awsSdk.String("iam:CreateUser"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeExplicitDeny},
					{EvalActionName: awsSdk.String("iam:DeleteAccessKey"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeImplicitDeny},
				},
			},
			expectedErr:    true,
			expectedErrMsg: "insufficient permissions for secret rotation. Denied actions: [iam:CreateUser iam:DeleteAccessKey]",
		},
		{
			name:           "simulate_api_error",
			accountID:      "123456789012",
			username:       "osdManagedAdmin-test",
			mockResponse:   nil,
			mockError:      assert.AnError,
			expectedErr:    true,
			expectedErrMsg: "failed to simulate principal policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mock_aws.NewMockClient(ctrl)

			expectedArn := "arn:aws:iam::" + tt.accountID + ":user/" + tt.username
			mockClient.EXPECT().
				SimulatePrincipalPolicy(gomock.Any()).
				DoAndReturn(func(input *iam.SimulatePrincipalPolicyInput) (*iam.SimulatePrincipalPolicyOutput, error) {
					assert.Equal(t, expectedArn, *input.PolicySourceArn)
					assert.Len(t, input.ActionNames, 10)
					return tt.mockResponse, tt.mockError
				}).
				Times(1)

			out := &bytes.Buffer{}
			err := VerifyRotationPermissions(out, mockClient, tt.accountID, tt.username)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRotateSecret_ExplicitAdminUsername(t *testing.T) {
	origInterval := SyncPollInterval
	origRetries := SyncMaxRetries
	SyncPollInterval = 0
	SyncMaxRetries = 1
	defer func() {
		SyncPollInterval = origInterval
		SyncMaxRetries = origRetries
	}()

	account := testAccount(false, false)
	secrets := testSecrets()
	cd := testClusterDeployment()
	cs := testClusterSync(true)

	objs := append(secrets, account, cd, cs)
	scheme := testScheme(t)
	kubeCli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithStatusSubresource(cs).Build()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	mockSimulateAllAllowed(mockClient)
	mockCreateAccessKey(mockClient, "osdManagedAdmin-custom")
	mockListAccessKeys(mockClient, "OLDKEY999", "NEWKEY123")

	out := &bytes.Buffer{}

	err := RotateSecret(context.Background(), &RotateSecretInput{
		AccountCRName:           "test-account",
		Account:                 account,
		OsdManagedAdminUsername: "osdManagedAdmin-custom",
		AwsClient:               mockClient,
		HiveKubeClient:          kubeCli,
		ManagedClusterClient:    testManagedClient(t),
		Out:                     out,
	})

	assert.NoError(t, err)
	assert.Contains(t, out.String(), "Successfully rotated secrets for osdManagedAdmin-custom")
}

func TestRotateSecret_DryRun(t *testing.T) {
	account := testAccount(false, false)
	secrets := testSecrets()

	objs := append(secrets, account)
	scheme := testScheme(t)
	kubeCli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	// Only SimulatePrincipalPolicy should be called (read-only validation).
	// No CreateAccessKey calls should happen.
	mockSimulateAllAllowed(mockClient)

	crIngress := &ccov1.CredentialsRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openshift-ingress",
			Namespace: credentialRequestNamespace,
		},
		Spec: ccov1.CredentialsRequestSpec{
			SecretRef:    corev1.ObjectReference{Name: "ingress-creds", Namespace: "openshift-ingress-operator"},
			ProviderSpec: awsProviderSpec(t),
		},
	}
	crCustom := &ccov1.CredentialsRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-cr",
			Namespace: credentialRequestNamespace,
		},
		Spec: ccov1.CredentialsRequestSpec{
			SecretRef:    corev1.ObjectReference{Name: "custom-creds", Namespace: "custom-ns"},
			ProviderSpec: awsProviderSpec(t),
		},
	}
	managedClient := testManagedClient(t, crIngress, crCustom)

	out := &bytes.Buffer{}

	err := RotateSecret(context.Background(), &RotateSecretInput{
		AccountCRName:        "test-account",
		Account:              account,
		DryRun:               true,
		AwsClient:            mockClient,
		HiveKubeClient:       kubeCli,
		ManagedClusterClient: managedClient,
		Out:                  out,
	})

	assert.NoError(t, err)
	output := out.String()
	assert.Contains(t, output, "[Dry Run] Would create a new IAM access key for user: osdManagedAdmin-abcd")
	assert.Contains(t, output, "[Dry Run] Would update secret aws-account-operator/test-account-secret")
	assert.Contains(t, output, "[Dry Run] Would update secret uhc-production-test/aws")
	assert.Contains(t, output, "[Dry Run] Would create SyncSet")
	assert.Contains(t, output, "[Dry Run] Would delete secret openshift-ingress-operator/ingress-creds (referenced by CredentialRequest openshift-ingress)")
	assert.NotContains(t, output, "custom-creds")
	assert.Contains(t, output, "[Dry Run] Would delete 1 credential secret(s) total")
	assert.Contains(t, output, "[Dry Run] No changes were made.")

	// Verify secrets were NOT modified
	secret := &corev1.Secret{}
	err = kubeCli.Get(context.Background(), types.NamespacedName{Name: "test-account-secret", Namespace: awsAccountNamespace}, secret)
	assert.NoError(t, err)
	assert.Equal(t, "old-key", string(secret.Data["aws_access_key_id"]))
}

func TestRotateSecret_DryRunWithCCS(t *testing.T) {
	account := testAccount(true, false)
	secrets := testSecrets()

	objs := append(secrets, account)
	scheme := testScheme(t)
	kubeCli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	mockSimulateAllAllowed(mockClient)

	managedClient := testManagedClient(t)

	out := &bytes.Buffer{}

	err := RotateSecret(context.Background(), &RotateSecretInput{
		AccountCRName:        "test-account",
		Account:              account,
		DryRun:               true,
		UpdateCcsCreds:       true,
		AwsClient:            mockClient,
		HiveKubeClient:       kubeCli,
		ManagedClusterClient: managedClient,
		Out:                  out,
	})

	assert.NoError(t, err)
	output := out.String()
	assert.Contains(t, output, "[Dry Run] Would create a new IAM access key for user: osdCcsAdmin")
	assert.Contains(t, output, "[Dry Run] Would update secret uhc-production-test/byoc")
	assert.Contains(t, output, "[Dry Run] No changes were made.")
}

func awsProviderSpec(t *testing.T) *runtime.RawExtension {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"kind": "AWSProviderSpec"})
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: raw}
}

func gcpProviderSpec(t *testing.T) *runtime.RawExtension {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"kind": "GCPProviderSpec"})
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: raw}
}

func TestRotateSecret_CredentialSecretDeletion(t *testing.T) {
	origInterval := SyncPollInterval
	origRetries := SyncMaxRetries
	SyncPollInterval = 0
	SyncMaxRetries = 1
	defer func() {
		SyncPollInterval = origInterval
		SyncMaxRetries = origRetries
	}()

	account := testAccount(false, false)
	secrets := testSecrets()
	cd := testClusterDeployment()
	cs := testClusterSync(true)

	objs := append(secrets, account, cd, cs)
	scheme := testScheme(t)
	kubeCli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithStatusSubresource(cs).Build()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	mockSimulateAllAllowed(mockClient)
	mockCreateAccessKey(mockClient, "osdManagedAdmin-abcd")
	mockListAccessKeys(mockClient, "OLDKEY999", "NEWKEY123")

	// Two AWS CredentialRequests with "openshift-" prefix → their secrets should be deleted.
	// One non-AWS CR with "openshift-" prefix → its secret should be kept.
	// One CR without the prefix → its secret should be kept.
	ingressSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ingress-creds", Namespace: "openshift-ingress-operator"},
	}
	machineAPISecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "machine-api-creds", Namespace: "openshift-machine-api"},
	}
	gcpSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "gcp-creds", Namespace: "openshift-gcp"},
	}
	customSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-creds", Namespace: "custom-ns"},
	}

	crIngress := &ccov1.CredentialsRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "openshift-ingress", Namespace: credentialRequestNamespace},
		Spec: ccov1.CredentialsRequestSpec{
			SecretRef:    corev1.ObjectReference{Name: "ingress-creds", Namespace: "openshift-ingress-operator"},
			ProviderSpec: awsProviderSpec(t),
		},
	}
	crMachineAPI := &ccov1.CredentialsRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "openshift-machine-api", Namespace: credentialRequestNamespace},
		Spec: ccov1.CredentialsRequestSpec{
			SecretRef:    corev1.ObjectReference{Name: "machine-api-creds", Namespace: "openshift-machine-api"},
			ProviderSpec: awsProviderSpec(t),
		},
	}
	crGCP := &ccov1.CredentialsRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "openshift-gcp-thing", Namespace: credentialRequestNamespace},
		Spec: ccov1.CredentialsRequestSpec{
			SecretRef:    corev1.ObjectReference{Name: "gcp-creds", Namespace: "openshift-gcp"},
			ProviderSpec: gcpProviderSpec(t),
		},
	}
	crCustom := &ccov1.CredentialsRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-cr", Namespace: credentialRequestNamespace},
		Spec: ccov1.CredentialsRequestSpec{
			SecretRef:    corev1.ObjectReference{Name: "custom-creds", Namespace: "custom-ns"},
			ProviderSpec: awsProviderSpec(t),
		},
	}

	managedClient := testManagedClient(t,
		ingressSecret, machineAPISecret, gcpSecret, customSecret,
		crIngress, crMachineAPI, crGCP, crCustom,
	)

	out := &bytes.Buffer{}

	err := RotateSecret(context.Background(), &RotateSecretInput{
		AccountCRName:        "test-account",
		Account:              account,
		AwsClient:            mockClient,
		HiveKubeClient:       kubeCli,
		ManagedClusterClient: managedClient,
		Out:                  out,
	})

	assert.NoError(t, err)
	output := out.String()
	assert.Contains(t, output, "Deleted secret openshift-ingress-operator/ingress-creds")
	assert.Contains(t, output, "Deleted secret openshift-machine-api/machine-api-creds")
	assert.NotContains(t, output, "gcp-creds")
	assert.NotContains(t, output, "custom-creds")
	assert.Contains(t, output, "Deleted 2 credential secret(s)")

	// Verify that the GCP and custom secrets still exist
	s := &corev1.Secret{}
	assert.NoError(t, managedClient.Get(context.Background(), types.NamespacedName{Name: "gcp-creds", Namespace: "openshift-gcp"}, s))
	assert.NoError(t, managedClient.Get(context.Background(), types.NamespacedName{Name: "custom-creds", Namespace: "custom-ns"}, s))

	// Verify all CredentialRequests still exist (they should not be deleted)
	remaining := &ccov1.CredentialsRequestList{}
	assert.NoError(t, managedClient.List(context.Background(), remaining, client.InNamespace(credentialRequestNamespace)))
	assert.Len(t, remaining.Items, 4)
}
