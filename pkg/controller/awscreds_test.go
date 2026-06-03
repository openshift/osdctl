package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ccov1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	mock_aws "github.com/openshift/osdctl/pkg/provider/aws/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func mockGetCallerIdentity(mockClient *mock_aws.MockClient) {
	mockClient.EXPECT().GetCallerIdentity(gomock.Any()).Return(&sts.GetCallerIdentityOutput{
		Arn:     awsSdk.String("arn:aws:sts::123456789012:assumed-role/test-role/session"),
		Account: awsSdk.String("123456789012"),
		UserId:  awsSdk.String("AROA1234:session"),
	}, nil).AnyTimes()
}

func diagInput(t *testing.T, mockClient *mock_aws.MockClient, hiveObjs []runtime.Object, managedObjs []runtime.Object) *AWSCredsInput {
	t.Helper()
	mockGetCallerIdentity(mockClient)
	account := testAccount(false, false)

	hiveClient := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithRuntimeObjects(append(hiveObjs, account)...).
		Build()

	var managedClient = testManagedClient(t, managedObjs...)

	return &AWSCredsInput{
		ClusterID:         "test-cluster-id",
		ClusterName:       "test-cluster",
		ClusterExternalID: "ext-12345",
		IsCCS:             false,
		AWSAccountID:      "123456789012",
		AccountCRName:     "test-account",
		Account:           account,
		AdminUsername:     "osdManagedAdmin-abcd",
		AwsClient:         mockClient,
		HiveKubeClient:    hiveClient,
		ManagedClient:     managedClient,
		Out:               &bytes.Buffer{},
	}
}

func TestDiagnoseCredentials_AllHealthy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	now := time.Now()
	keyCreated := now.Add(-24 * time.Hour)

	mockClient.EXPECT().ListAccessKeys(gomock.Any()).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIAEXAMPLE1234"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockSimulateAllAllowed(mockClient)

	secrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}

	input := diagInput(t, mockClient, secrets, nil)

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	assert.True(t, report.AllPermissionsOK)
	assert.True(t, report.AllSecretsInSync)
	assert.Len(t, report.Keys, 1)
	assert.True(t, report.Keys[0].HiveMatch)
	assert.Len(t, report.Secrets, 2)
	for _, s := range report.Secrets {
		assert.True(t, s.Exists)
		assert.True(t, s.MatchesAWS)
	}

	failFindings := 0
	for _, f := range report.Findings {
		if f.Severity == "FAIL" {
			failFindings++
		}
	}
	assert.Equal(t, 0, failFindings)
}

func TestDiagnoseCredentials_KeyMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	now := time.Now()
	keyCreated := now.Add(-24 * time.Hour)

	mockClient.EXPECT().ListAccessKeys(gomock.Any()).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIANEWKEY99999"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockSimulateAllAllowed(mockClient)

	secrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAOLDKEY00000")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAOLDKEY00000")},
		},
	}

	input := diagInput(t, mockClient, secrets, nil)

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	assert.False(t, report.AllSecretsInSync)

	hasMismatchFinding := false
	for _, f := range report.Findings {
		if f.Severity == "FAIL" && strings.Contains(f.Message, "does not match") {
			hasMismatchFinding = true
		}
	}
	assert.True(t, hasMismatchFinding, "expected a FAIL finding about key mismatch")
}

func TestDiagnoseCredentials_MaxKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	now := time.Now()
	oldKey := now.Add(-90 * 24 * time.Hour)
	newKey := now.Add(-1 * 24 * time.Hour)

	mockClient.EXPECT().ListAccessKeys(gomock.Any()).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIAOLDKEY00000"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &oldKey,
			},
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIANEWKEY99999"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &newKey,
			},
		},
	}, nil)

	mockSimulateAllAllowed(mockClient)

	secrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIANEWKEY99999")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIANEWKEY99999")},
		},
	}

	input := diagInput(t, mockClient, secrets, nil)

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	assert.Len(t, report.Keys, 2)

	hasMaxKeyInfo := false
	for _, f := range report.Findings {
		if f.Severity == "INFO" && strings.Contains(f.Message, "max 2") {
			hasMaxKeyInfo = true
		}
	}
	assert.True(t, hasMaxKeyInfo, "expected INFO finding about max keys")
}

func TestDiagnoseCredentials_PermissionDenied(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	now := time.Now()
	keyCreated := now.Add(-24 * time.Hour)

	mockClient.EXPECT().ListAccessKeys(gomock.Any()).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIAEXAMPLE1234"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockClient.EXPECT().SimulatePrincipalPolicy(gomock.Any()).Return(&iam.SimulatePrincipalPolicyOutput{
		EvaluationResults: []iamTypes.EvaluationResult{
			{EvalActionName: awsSdk.String("iam:CreateAccessKey"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
			{EvalActionName: awsSdk.String("iam:DeleteAccessKey"), EvalDecision: "implicitDeny"},
			{EvalActionName: awsSdk.String("iam:ListAccessKeys"), EvalDecision: iamTypes.PolicyEvaluationDecisionTypeAllowed},
		},
	}, nil)

	secrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}

	input := diagInput(t, mockClient, secrets, nil)

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	assert.False(t, report.AllPermissionsOK)

	hasPermFinding := false
	for _, f := range report.Findings {
		if f.Severity == "FAIL" && strings.Contains(f.Message, "Insufficient IAM permissions") {
			hasPermFinding = true
			assert.Contains(t, f.Message, "iam:DeleteAccessKey")
		}
	}
	assert.True(t, hasPermFinding, "expected FAIL finding about denied permissions")
}

func TestDiagnoseCredentials_MissingSecret(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	now := time.Now()
	keyCreated := now.Add(-24 * time.Hour)

	mockClient.EXPECT().ListAccessKeys(gomock.Any()).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIAEXAMPLE1234"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockSimulateAllAllowed(mockClient)

	// Only provide one secret — the "aws" secret in the CD namespace is missing
	secrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}

	input := diagInput(t, mockClient, secrets, nil)

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	assert.False(t, report.AllSecretsInSync)

	hasMissingFinding := false
	for _, f := range report.Findings {
		if f.Severity == "FAIL" && strings.Contains(f.Message, "not found") {
			hasMissingFinding = true
		}
	}
	assert.True(t, hasMissingFinding, "expected FAIL finding about missing secret")
}

func TestDiagnoseCredentials_WithCredentialRequests(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	now := time.Now()
	keyCreated := now.Add(-24 * time.Hour)

	mockClient.EXPECT().ListAccessKeys(gomock.Any()).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIAEXAMPLE1234"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockSimulateAllAllowed(mockClient)

	hiveSecrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}

	providerSpec := mustMarshal(t, map[string]string{"kind": "AWSProviderSpec"})
	managedObjs := []runtime.Object{
		&ccov1.CredentialsRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openshift-image-registry",
				Namespace: credentialRequestNamespace,
			},
			Spec: ccov1.CredentialsRequestSpec{
				SecretRef: corev1.ObjectReference{
					Name:      "installer-cloud-credentials",
					Namespace: "openshift-image-registry",
				},
				ProviderSpec: &runtime.RawExtension{Raw: providerSpec},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "installer-cloud-credentials",
				Namespace:         "openshift-image-registry",
				CreationTimestamp: metav1.NewTime(now.Add(-48 * time.Hour)),
			},
		},
	}

	input := diagInput(t, mockClient, hiveSecrets, managedObjs)

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	assert.Len(t, report.CredRequests, 1)
	assert.True(t, report.CredRequests[0].Exists)
	assert.Equal(t, "openshift-image-registry", report.CredRequests[0].CredRequestName)
}

func TestDiagnoseCredentials_MissingCredentialSecret(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	now := time.Now()
	keyCreated := now.Add(-24 * time.Hour)

	mockClient.EXPECT().ListAccessKeys(gomock.Any()).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIAEXAMPLE1234"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockSimulateAllAllowed(mockClient)

	hiveSecrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}

	providerSpec := mustMarshal(t, map[string]string{"kind": "AWSProviderSpec"})
	managedObjs := []runtime.Object{
		&ccov1.CredentialsRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openshift-ingress-operator",
				Namespace: credentialRequestNamespace,
			},
			Spec: ccov1.CredentialsRequestSpec{
				SecretRef: corev1.ObjectReference{
					Name:      "cloud-credentials",
					Namespace: "openshift-ingress-operator",
				},
				ProviderSpec: &runtime.RawExtension{Raw: providerSpec},
			},
		},
		// No secret created — simulates missing credential secret
	}

	input := diagInput(t, mockClient, hiveSecrets, managedObjs)

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	assert.Len(t, report.CredRequests, 1)
	assert.False(t, report.CredRequests[0].Exists)

	hasWarn := false
	for _, f := range report.Findings {
		if f.Severity == "WARN" && strings.Contains(f.Message, "cloud-credentials") {
			hasWarn = true
			assert.Contains(t, f.Guidance, "CCO")
		}
	}
	assert.True(t, hasWarn, "expected WARN about missing credential secret")
}

func TestDiagnoseCredentials_CCSCluster(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)
	mockGetCallerIdentity(mockClient)

	now := time.Now()
	keyCreated := now.Add(-24 * time.Hour)

	// Expect ListAccessKeys called for both osdManagedAdmin and osdCcsAdmin
	mockClient.EXPECT().ListAccessKeys(&iam.ListAccessKeysInput{
		UserName: awsSdk.String("osdManagedAdmin-abcd"),
	}).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIAMANAGED0001"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockClient.EXPECT().ListAccessKeys(&iam.ListAccessKeysInput{
		UserName: awsSdk.String("osdCcsAdmin"),
	}).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdCcsAdmin"),
				AccessKeyId: awsSdk.String("AKIACCSADM00001"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockSimulateAllAllowed(mockClient)

	account := testAccount(true, false)
	hiveObjs := []runtime.Object{
		account,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAMANAGED0001")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAMANAGED0001")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "byoc", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIACCSADM00001")},
		},
	}

	hiveClient := fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(hiveObjs...).Build()
	managedClient := testManagedClient(t)

	input := &AWSCredsInput{
		ClusterID:         "test-cluster-id",
		ClusterName:       "test-ccs-cluster",
		ClusterExternalID: "ext-ccs-12345",
		IsCCS:             true,
		AWSAccountID:      "123456789012",
		AccountCRName:     "test-account",
		Account:           account,
		AdminUsername:     "osdManagedAdmin-abcd",
		AwsClient:         mockClient,
		HiveKubeClient:    hiveClient,
		ManagedClient:     managedClient,
		Out:               &bytes.Buffer{},
	}

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	assert.Equal(t, "osdCcsAdmin", report.CcsAdminUser)
	assert.Len(t, report.Secrets, 3) // account-secret, aws, byoc
	assert.Len(t, report.Keys, 2)    // managed + ccs
	assert.True(t, report.AllSecretsInSync)
	assert.True(t, report.AllPermissionsOK)
}

func TestRenderReport_OutputFormat(t *testing.T) {
	report := &DiagnosticReport{
		ClusterID:         "abc123",
		ClusterName:       "my-cluster",
		ClusterExternalID: "ext-99999",
		IsCCS:             true,
		AWSAccountID:      "123456789012",
		AccountCRName:     "test-account",
		ManagedAdminUser:  "osdManagedAdmin-abcd",
		CcsAdminUser:      "osdCcsAdmin",
		Keys: []KeyStatus{
			{UserName: "osdManagedAdmin-abcd", AccessKeyID: "AKIAEXAMPLE1234", Age: 48 * time.Hour, Status: "Active", HiveMatch: true},
		},
		Secrets: []SecretStatus{
			{SecretName: "test-account-secret", Namespace: "aws-account-operator", AccessKeyID: "AKIAEXAMPLE1234", Exists: true, MatchesAWS: true},
			{SecretName: "aws", Namespace: "uhc-production-test", AccessKeyID: "AKIAEXAMPLE1234", Exists: true, MatchesAWS: true},
		},
		CredRequests: []CredRequestStatus{
			{CredRequestName: "openshift-image-registry", SecretName: "cloud-creds", Namespace: "openshift-image-registry", Age: 48 * time.Hour, Exists: true},
		},
		Permissions:      []PermissionResult{{Action: "iam:CreateAccessKey", Allowed: true}},
		AllPermissionsOK: true,
		AllSecretsInSync: true,
	}

	var buf bytes.Buffer
	RenderReport(report, &buf)
	output := buf.String()

	assert.Contains(t, output, "AWS Account: 123456789012")
	assert.Contains(t, output, "Account CR: test-account")
	assert.Contains(t, output, "AWS Credentials")
	assert.Contains(t, output, "REF'd by HIVE SECRET(S)")
	assert.Contains(t, output, "Credential Request Secrets")
	assert.Contains(t, output, "IAM Permission Check")
	assert.Contains(t, output, "No issues found during pre-rotation checks")
}

func TestRenderReport_WithFindings(t *testing.T) {
	report := &DiagnosticReport{
		ClusterID:         "abc123",
		ClusterName:       "my-cluster",
		ClusterExternalID: "ext-99999",
		IsCCS:             false,
		AWSAccountID:      "123456789012",
		AccountCRName:     "test-account",
		ManagedAdminUser:  "osdManagedAdmin-abcd",
		Keys:              []KeyStatus{},
		Secrets:           []SecretStatus{},
		AllPermissionsOK:  true,
		AllSecretsInSync:  false,
		Findings: []Finding{
			{Severity: "FAIL", Message: "Hive secret aws/uhc-production-test not found", Guidance: "Check hive namespace"},
			{Severity: "WARN", Message: "osdManagedAdmin-abcd has 2 access keys (max 2)", Guidance: "Delete the old key"},
		},
	}

	var buf bytes.Buffer
	RenderReport(report, &buf)
	output := buf.String()

	assert.Contains(t, output, "[FAIL]")
	assert.Contains(t, output, "[WARN]")
	assert.Contains(t, output, "Hive secret aws/uhc-production-test not found")
	assert.Contains(t, output, "2 access keys (max 2)")
}

func TestFormatAge(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{48 * time.Hour, "2d"},
		{3 * time.Hour, "3h"},
		{30 * time.Minute, "30m"},
		{90 * 24 * time.Hour, "90d"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, formatAge(tt.duration))
	}
}

func TestTruncateKeyID(t *testing.T) {
	assert.Equal(t, "AKIA...1234", truncateKeyID("AKIAEXAMPLE1234"))
	assert.Equal(t, "short", truncateKeyID("short"))
	assert.Equal(t, "", truncateKeyID(""))
}

func TestDiagnoseCredentials_AdminUsernameFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)
	mockGetCallerIdentity(mockClient)

	now := time.Now()
	keyCreated := now.Add(-24 * time.Hour)

	nse := &iamTypes.NoSuchEntityException{Message: awsSdk.String("user not found")}
	mockClient.EXPECT().ListAccessKeys(&iam.ListAccessKeysInput{
		UserName: awsSdk.String("osdManagedAdmin-abcd"),
	}).Return(nil, nse)

	mockClient.EXPECT().ListAccessKeys(&iam.ListAccessKeysInput{
		UserName: awsSdk.String("osdManagedAdmin"),
	}).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin"),
				AccessKeyId: awsSdk.String("AKIAEXAMPLE1234"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockSimulateAllAllowed(mockClient)

	secrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}

	input := diagInput(t, mockClient, secrets, nil)

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	assert.Equal(t, "osdManagedAdmin", report.ManagedAdminUser)
	assert.Len(t, report.Keys, 1)
	assert.True(t, report.AllPermissionsOK)
}

func TestDiagnoseCredentials_AdminUsernameBothNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)
	mockGetCallerIdentity(mockClient)

	nse := &iamTypes.NoSuchEntityException{Message: awsSdk.String("user not found")}
	mockClient.EXPECT().ListAccessKeys(gomock.Any()).Return(nil, nse).Times(2)

	mockSimulateAllAllowed(mockClient)

	secrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}

	input := diagInput(t, mockClient, secrets, nil)

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	hasFinding := false
	for _, f := range report.Findings {
		if f.Severity == "FAIL" && strings.Contains(f.Message, "not found") {
			hasFinding = true
			assert.Contains(t, f.Guidance, "--admin-username")
		}
	}
	assert.True(t, hasFinding, "expected FAIL finding about admin user not found")
}

func TestDiagnoseCredentials_NilManagedClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)
	mockGetCallerIdentity(mockClient)

	now := time.Now()
	keyCreated := now.Add(-24 * time.Hour)

	mockClient.EXPECT().ListAccessKeys(gomock.Any()).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIAEXAMPLE1234"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockSimulateAllAllowed(mockClient)

	account := testAccount(false, false)
	hiveObjs := []runtime.Object{
		account,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}

	hiveClient := fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(hiveObjs...).Build()

	input := &AWSCredsInput{
		ClusterID:         "test-cluster-id",
		ClusterName:       "test-cluster",
		ClusterExternalID: "ext-12345",
		IsCCS:             false,
		AWSAccountID:      "123456789012",
		AccountCRName:     "test-account",
		Account:           account,
		AdminUsername:     "osdManagedAdmin-abcd",
		AwsClient:         mockClient,
		HiveKubeClient:    hiveClient,
		ManagedClient:     nil,
		Out:               &bytes.Buffer{},
	}

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	assert.Empty(t, report.CredRequests)
	assert.True(t, report.AllPermissionsOK)
}

func TestDiagnoseCredentials_StaleCredentialSecrets(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	now := time.Now()
	keyCreated := now.Add(-1 * time.Hour)

	mockClient.EXPECT().ListAccessKeys(gomock.Any()).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIAEXAMPLE1234"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockSimulateAllAllowed(mockClient)

	hiveSecrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}

	providerSpec := mustMarshal(t, map[string]string{"kind": "AWSProviderSpec"})
	managedObjs := []runtime.Object{
		&ccov1.CredentialsRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openshift-image-registry",
				Namespace: "openshift-cloud-credential-operator",
			},
			Spec: ccov1.CredentialsRequestSpec{
				SecretRef: corev1.ObjectReference{
					Name:      "cloud-credentials",
					Namespace: "openshift-image-registry",
				},
				ProviderSpec: &runtime.RawExtension{Raw: providerSpec},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "cloud-credentials",
				Namespace:         "openshift-image-registry",
				CreationTimestamp: metav1.NewTime(now.Add(-30 * 24 * time.Hour)),
			},
		},
	}

	input := diagInput(t, mockClient, hiveSecrets, managedObjs)

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	require.Len(t, report.CredRequests, 1)
	assert.True(t, report.CredRequests[0].NeedsRecreation, "secret older than key should be marked NEEDS REFRESH")
}

func TestDiagnoseCredentials_CallerIdentityInReport(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	mockClient.EXPECT().GetCallerIdentity(gomock.Any()).Return(&sts.GetCallerIdentityOutput{
		Arn:     awsSdk.String("arn:aws:sts::123456789012:assumed-role/ManagedOpenShift-Support-abcd/session"),
		Account: awsSdk.String("123456789012"),
		UserId:  awsSdk.String("AROA1234:session"),
	}, nil)

	now := time.Now()
	keyCreated := now.Add(-24 * time.Hour)

	mockClient.EXPECT().ListAccessKeys(gomock.Any()).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIAEXAMPLE1234"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockSimulateAllAllowed(mockClient)

	secrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}

	account := testAccount(false, false)
	hiveClient := fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(append(secrets, account)...).Build()

	input := &AWSCredsInput{
		ClusterID:         "test-cluster-id",
		ClusterName:       "test-cluster",
		ClusterExternalID: "ext-12345",
		IsCCS:             false,
		AWSAccountID:      "123456789012",
		AccountCRName:     "test-account",
		Account:           account,
		AdminUsername:     "osdManagedAdmin-abcd",
		AwsClient:         mockClient,
		HiveKubeClient:    hiveClient,
		ManagedClient:     testManagedClient(t),
		Out:               &bytes.Buffer{},
	}

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	assert.Equal(t, "arn:aws:sts::123456789012:assumed-role/ManagedOpenShift-Support-abcd/session", report.CallerARN)
	assert.Equal(t, "123456789012", report.CallerAccount)

	var buf bytes.Buffer
	RenderReport(report, &buf)
	assert.Contains(t, buf.String(), "ManagedOpenShift-Support-abcd")
}

func TestDiagnoseCredentials_CredReqNamespaceFilter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)

	now := time.Now()
	keyCreated := now.Add(-24 * time.Hour)

	mockClient.EXPECT().ListAccessKeys(gomock.Any()).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIAEXAMPLE1234"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockSimulateAllAllowed(mockClient)

	hiveSecrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}

	providerSpec := mustMarshal(t, map[string]string{"kind": "AWSProviderSpec"})
	managedObjs := []runtime.Object{
		// Should be included: openshift namespace prefix
		&ccov1.CredentialsRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "aws-ebs-csi", Namespace: "openshift-cluster-csi-drivers"},
			Spec: ccov1.CredentialsRequestSpec{
				SecretRef:    corev1.ObjectReference{Name: "ebs-creds", Namespace: "openshift-cluster-csi-drivers"},
				ProviderSpec: &runtime.RawExtension{Raw: providerSpec},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "ebs-creds", Namespace: "openshift-cluster-csi-drivers",
				CreationTimestamp: metav1.NewTime(now.Add(-1 * time.Hour))},
		},
		// Should be excluded: non-openshift namespace
		&ccov1.CredentialsRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "customer-cr", Namespace: "customer-namespace"},
			Spec: ccov1.CredentialsRequestSpec{
				SecretRef:    corev1.ObjectReference{Name: "customer-secret", Namespace: "customer-namespace"},
				ProviderSpec: &runtime.RawExtension{Raw: providerSpec},
			},
		},
		// Should be excluded: GCP provider
		&ccov1.CredentialsRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "gcp-cr", Namespace: "openshift-cloud-credential-operator"},
			Spec: ccov1.CredentialsRequestSpec{
				SecretRef:    corev1.ObjectReference{Name: "gcp-secret", Namespace: "openshift-gcp"},
				ProviderSpec: &runtime.RawExtension{Raw: mustMarshal(t, map[string]string{"kind": "GCPProviderSpec"})},
			},
		},
	}

	input := diagInput(t, mockClient, hiveSecrets, managedObjs)

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	assert.Len(t, report.CredRequests, 1, "should only include AWS CRs in openshift-* namespaces")
	assert.Equal(t, "aws-ebs-csi", report.CredRequests[0].CredRequestName)
}

func TestDiagnoseCredentials_SimulateAPIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := mock_aws.NewMockClient(ctrl)
	mockGetCallerIdentity(mockClient)

	now := time.Now()
	keyCreated := now.Add(-24 * time.Hour)

	mockClient.EXPECT().ListAccessKeys(gomock.Any()).Return(&iam.ListAccessKeysOutput{
		AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
			{
				UserName:    awsSdk.String("osdManagedAdmin-abcd"),
				AccessKeyId: awsSdk.String("AKIAEXAMPLE1234"),
				Status:      iamTypes.StatusTypeActive,
				CreateDate:  &keyCreated,
			},
		},
	}, nil)

	mockClient.EXPECT().SimulatePrincipalPolicy(gomock.Any()).Return(nil, fmt.Errorf("access denied")).AnyTimes()

	secrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-account-secret", Namespace: awsAccountNamespace},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "uhc-production-test"},
			Data:       map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}

	account := testAccount(false, false)
	hiveClient := fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(append(secrets, account)...).Build()

	input := &AWSCredsInput{
		ClusterID:         "test-cluster-id",
		ClusterName:       "test-cluster",
		ClusterExternalID: "ext-12345",
		IsCCS:             false,
		AWSAccountID:      "123456789012",
		AccountCRName:     "test-account",
		Account:           account,
		AdminUsername:     "osdManagedAdmin-abcd",
		AwsClient:         mockClient,
		HiveKubeClient:    hiveClient,
		ManagedClient:     testManagedClient(t),
		Out:               &bytes.Buffer{},
	}

	report, err := DiagnoseCredentials(context.TODO(), input)
	require.NoError(t, err)

	assert.False(t, report.AllPermissionsOK)

	hasWarn := false
	for _, f := range report.Findings {
		if f.Severity == "WARN" && strings.Contains(f.Message, "Failed to simulate") {
			hasWarn = true
		}
	}
	assert.True(t, hasWarn, "expected WARN about simulate API failure")
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestDiagnoseCRSecrets_Healthy(t *testing.T) {
	now := time.Now()
	account := testAccount(false, false)

	hiveObjs := []runtime.Object{
		account,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-account-secret",
				Namespace:         awsAccountNamespace,
				CreationTimestamp: metav1.NewTime(now.Add(-1 * time.Hour)),
			},
			Data: map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}
	hiveClient := fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(hiveObjs...).Build()

	providerSpec := mustMarshal(t, map[string]string{"kind": "AWSProviderSpec"})
	managedObjs := []runtime.Object{
		&ccov1.CredentialsRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "openshift-ingress", Namespace: "openshift-cloud-credential-operator"},
			Spec: ccov1.CredentialsRequestSpec{
				SecretRef:    corev1.ObjectReference{Name: "cloud-creds", Namespace: "openshift-ingress-operator"},
				ProviderSpec: &runtime.RawExtension{Raw: providerSpec},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "cloud-creds",
				Namespace:         "openshift-ingress-operator",
				CreationTimestamp: metav1.NewTime(now.Add(-30 * time.Minute)),
			},
		},
	}
	managedClient := testManagedClient(t, managedObjs...)

	var buf bytes.Buffer
	report, err := DiagnoseCRSecrets(context.TODO(), hiveClient, managedClient, "test-account", account, &buf)
	require.NoError(t, err)

	require.Len(t, report.CredRequests, 1)
	assert.True(t, report.CredRequests[0].Exists)
	assert.False(t, report.CredRequests[0].NeedsRecreation, "secret newer than account secret should be CURRENT")
}

func TestDiagnoseCRSecrets_Stale(t *testing.T) {
	now := time.Now()
	account := testAccount(false, false)

	hiveObjs := []runtime.Object{
		account,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-account-secret",
				Namespace:         awsAccountNamespace,
				CreationTimestamp: metav1.NewTime(now.Add(-1 * time.Hour)),
			},
			Data: map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}
	hiveClient := fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(hiveObjs...).Build()

	providerSpec := mustMarshal(t, map[string]string{"kind": "AWSProviderSpec"})
	managedObjs := []runtime.Object{
		&ccov1.CredentialsRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "openshift-ingress", Namespace: "openshift-cloud-credential-operator"},
			Spec: ccov1.CredentialsRequestSpec{
				SecretRef:    corev1.ObjectReference{Name: "cloud-creds", Namespace: "openshift-ingress-operator"},
				ProviderSpec: &runtime.RawExtension{Raw: providerSpec},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "cloud-creds",
				Namespace:         "openshift-ingress-operator",
				CreationTimestamp: metav1.NewTime(now.Add(-7 * 24 * time.Hour)),
			},
		},
	}
	managedClient := testManagedClient(t, managedObjs...)

	var buf bytes.Buffer
	report, err := DiagnoseCRSecrets(context.TODO(), hiveClient, managedClient, "test-account", account, &buf)
	require.NoError(t, err)

	require.Len(t, report.CredRequests, 1)
	assert.True(t, report.CredRequests[0].NeedsRecreation, "secret older than account secret should need refresh")
}

func TestDiagnoseCRSecrets_MissingSecret(t *testing.T) {
	now := time.Now()
	account := testAccount(false, false)

	hiveObjs := []runtime.Object{
		account,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-account-secret",
				Namespace:         awsAccountNamespace,
				CreationTimestamp: metav1.NewTime(now.Add(-1 * time.Hour)),
			},
			Data: map[string][]byte{"aws_access_key_id": []byte("AKIAEXAMPLE1234")},
		},
	}
	hiveClient := fake.NewClientBuilder().WithScheme(testScheme(t)).WithRuntimeObjects(hiveObjs...).Build()

	providerSpec := mustMarshal(t, map[string]string{"kind": "AWSProviderSpec"})
	managedObjs := []runtime.Object{
		&ccov1.CredentialsRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "openshift-ingress", Namespace: "openshift-cloud-credential-operator"},
			Spec: ccov1.CredentialsRequestSpec{
				SecretRef:    corev1.ObjectReference{Name: "cloud-creds", Namespace: "openshift-ingress-operator"},
				ProviderSpec: &runtime.RawExtension{Raw: providerSpec},
			},
		},
	}
	managedClient := testManagedClient(t, managedObjs...)

	var buf bytes.Buffer
	report, err := DiagnoseCRSecrets(context.TODO(), hiveClient, managedClient, "test-account", account, &buf)
	require.NoError(t, err)

	require.Len(t, report.CredRequests, 1)
	assert.False(t, report.CredRequests[0].Exists)
}
