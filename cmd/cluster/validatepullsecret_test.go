package cluster

import (
	"fmt"
	"reflect"
	"testing"

	sdk "github.com/openshift-online/ocm-sdk-go"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	v1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
)

func Test_getPullSecretEmail(t *testing.T) {
	tests := []struct {
		name          string
		secret        *corev1.Secret
		expectedEmail string
		expectedError error
		expectedDone  bool
	}{
		{
			name:         "Missing dockerconfigjson",
			secret:       &corev1.Secret{Data: map[string][]byte{}},
			expectedDone: true,
		},
		{
			name:         "Missing cloud.openshift.com auth",
			secret:       &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{}}")}},
			expectedDone: true,
		},
		{
			name:         "Missing email",
			secret:       &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{\"cloud.openshift.com\":{}}}")}},
			expectedDone: true,
		},
		{
			name:          "Valid pull secret",
			secret:        &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{\"cloud.openshift.com\":{\"email\":\"foo@bar.com\"}}}")}},
			expectedEmail: "foo@bar.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, err, done := getPullSecretEmail("abc123", tt.secret, false)
			if email != tt.expectedEmail {
				t.Errorf("getPullSecretEmail() email = %v, expectedEmail %v", email, tt.expectedEmail)
			}
			if !reflect.DeepEqual(err, tt.expectedError) {
				t.Errorf("getPullSecretEmail() err = %v, expectedEmail %v", err, tt.expectedError)
			}
			if done != tt.expectedDone {
				t.Errorf("getPullSecretEmail() done = %v, expectedEmail %v", done, tt.expectedDone)
			}
		})
	}
}

// covering only upto get subscription need to check again
// func TestGetPullSecretFromOCM(t *testing.T) {
// 	tests := []struct {
// 		name          string
// 		o             *validatePullSecretOptions
// 		expectErr     bool
// 		expectEmail   string
// 		expectSuccess bool
// 	}{
// 		{
// 			name: "Connection creation fails",
// 			o: &validatePullSecretOptions{
// 				clusterID: "clusterabc",
// 			},
// 			expectErr:     true,
// 			expectEmail:   "",
// 			expectSuccess: false,
// 		},
// 		{
// 			name:          "Subscription fetch fails",
// 			o:             &validatePullSecretOptions{},
// 			expectErr:     true,
// 			expectEmail:   "",
// 			expectSuccess: false,
// 		},
// 		{
// 			name:          "No registry credentials found, service log posted",
// 			o:             &validatePullSecretOptions{},
// 			expectErr:     false,
// 			expectEmail:   "",
// 			expectSuccess: true,
// 		},
// 		{
// 			name:          "Successful fetch of email",
// 			o:             &validatePullSecretOptions{},
// 			expectErr:     false,
// 			expectEmail:   "", // Replace with expected email
// 			expectSuccess: false,
// 		},
// 	}

// 	for _, test := range tests {
// 		t.Run(test.name, func(t *testing.T) {
// 			// Run the function with the test's 'o' struct
// 			email, err, success := test.o.getPullSecretFromOCM()

// 			if err != nil {
// 				return
// 			}

// 			// Check if we expect an error
// 			if test.expectErr {
// 				assert.Error(t, err)
// 			} else {
// 				assert.NoError(t, err)
// 			}

// 			// Assert the email value
// 			assert.Equal(t, test.expectEmail, email)

// 			// // Assert the success flag
// 			assert.Equal(t, test.expectSuccess, success)
// 		})
// 	}
// }

//==========================================================

type MockPullSecretFetcher struct {
	mock.Mock
}

func (m *MockPullSecretFetcher) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockPullSecretFetcher) getPullSecretFromOCM() (string, error, bool) {

	args := m.Called()
	return args.String(0), args.Error(1), args.Bool(2)
}

type MockClusterPullSecretFetcher struct {
	mock.Mock
}

func (m *MockClusterPullSecretFetcher) getPullSecretElevated(clusterID string, reason string) (string, error, bool) {
	args := m.Called(clusterID, reason)
	return args.String(0), args.Error(1), args.Bool(2)
}

func TestRun(t *testing.T) {
	// Define test cases
	testCases := []struct {
		name              string
		ocmEmail          string
		clusterEmail      string
		ocmErr            error
		clusterErr        error
		expectedError     string
		expectErrorOutput bool
	}{
		{
			name:              "ValidPullSecret",
			ocmEmail:          "user@example.com",
			clusterEmail:      "user@example.com",
			ocmErr:            nil,
			clusterErr:        nil,
			expectedError:     "",
			expectErrorOutput: false,
		},
		{
			name:              "EmailMismatch",
			ocmEmail:          "user@example.com",
			clusterEmail:      "otheruser@example.com",
			ocmErr:            nil,
			clusterErr:        nil,
			expectedError:     "",
			expectErrorOutput: true,
		},
	}

	// Iterate through test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Directly create and configure mock objects for OCM and cluster fetchers
			mockOCMFetcher := new(MockPullSecretFetcher)
			mockClusterFetcher := new(MockClusterPullSecretFetcher)

			// Setup expectations for OCM fetch
			mockOCMFetcher.On("getPullSecretFromOCM").Return(tc.ocmEmail, tc.ocmErr, false)

			// Setup expectations for cluster fetch
			mockClusterFetcher.On("getPullSecretElevated", "abcCluster", "reason").Return(tc.clusterEmail, tc.clusterErr, false)

			// Create the validatePullSecretOptions object with mock dependencies
			options := &validatePullSecretOptions{
				clusterID:            "abcCluster",
				reason:               "reason",
				pullSecretFetcher:    mockOCMFetcher,
				clusterSecretFetcher: mockClusterFetcher,
				oCMClientInterface:   &OCMClientImpl{},
			}

			// Run the function
			err := options.run()

			// Assert that the error is as expected
			if tc.expectedError != "" {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError, err.Error())

			}

			// Assert that the mocks were called as expected
			mockOCMFetcher.AssertExpectations(t)
			mockClusterFetcher.AssertExpectations(t)
		})
	}
}

// ============================================================================================================
// ============================================================================================================
// type MockOCMClient struct {
// 	mock.Mock
// }

func (m *MockPullSecretFetcher) CreateConnection() (*sdk.Connection, error) {
	args := m.Called()
	return args.Get(0).(*sdk.Connection), args.Error(1)
}

func (m *MockPullSecretFetcher) GetSubscription(connection *sdk.Connection, key string) (*amv1.Subscription, error) {
	args := m.Called(connection, key)
	return args.Get(0).(*amv1.Subscription), args.Error(1)
}

func (m *MockPullSecretFetcher) GetAccount(connection *sdk.Connection, key string) (*amv1.Account, error) {
	args := m.Called(connection, key)
	return args.Get(0).(*amv1.Account), args.Error(1)
}

func (m *MockPullSecretFetcher) GetRegistryCredentials(connection *sdk.Connection, key string) ([]*amv1.RegistryCredential, error) {
	args := m.Called(connection, key)
	return args.Get(0).([]*amv1.RegistryCredential), args.Error(1)
}

func (m *MockPullSecretFetcher) Email() string {
	args := m.Called()
	return args.String(0)
}

func TestGetPullSecretFromOCM(t *testing.T) {
	// Define test cases
	testCases := []struct {
		name             string
		createConnErr    error
		getSubErr        error
		getAccErr        error
		getRegCredsErr   error
		registryCreds    []*amv1.RegistryCredential
		expectedEmail    string
		expectedError    string
		expectServiceLog bool
	}{
		{
			name:             "SuccessfulFetch",
			createConnErr:    nil,
			getSubErr:        nil,
			getAccErr:        nil,
			getRegCredsErr:   nil,
			registryCreds:    []*amv1.RegistryCredential{{}}, // Assume credentials exist
			expectedEmail:    "user@example.com",
			expectedError:    "",
			expectServiceLog: false,
		},
		{
			name:             "ErrorCreatingConnection",
			createConnErr:    fmt.Errorf("Connection error"),
			getSubErr:        nil,
			getAccErr:        nil,
			getRegCredsErr:   nil,
			registryCreds:    nil,
			expectedEmail:    "user@example.com",
			expectedError:    "Connection error",
			expectServiceLog: false,
		},
		{
			name:             "ErrorFetchingSubscription",
			createConnErr:    nil,
			getSubErr:        fmt.Errorf("Subscription fetch error"),
			getAccErr:        nil,
			getRegCredsErr:   nil,
			registryCreds:    nil,
			expectedEmail:    "user@example.com",
			expectedError:    "Subscription fetch error",
			expectServiceLog: false,
		},
		{
			name:             "ErrorFetchingAccount",
			createConnErr:    nil,
			getSubErr:        nil,
			getAccErr:        fmt.Errorf("Account fetch error"),
			getRegCredsErr:   nil,
			registryCreds:    nil,
			expectedEmail:    "user@example.com",
			expectedError:    "Account fetch error",
			expectServiceLog: false,
		},
		{
			name:             "NoRegistryCredentialsFound",
			createConnErr:    nil,
			getSubErr:        nil,
			getAccErr:        nil,
			getRegCredsErr:   nil,
			registryCreds:    nil, // No credentials
			expectedEmail:    "user@example.com",
			expectedError:    "",
			expectServiceLog: true,
		},
	}

	// Iterate through test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mocks
			mockOCMClient := new(MockPullSecretFetcher)
			account, _ := v1.NewAccount().Email("user@example.com").Build()

			// Mock methods for the test case
			// mockAccount.On("Email").Return(tc.expectedEmail)
			mockOCMClient.On("CreateConnection").Return(&sdk.Connection{}, tc.createConnErr)
			mockOCMClient.On("GetSubscription", mock.Anything, mock.Anything).Return(&amv1.Subscription{}, tc.getSubErr)
			mockOCMClient.On("GetAccount", mock.Anything, mock.Anything).Return(account, tc.getAccErr)
			mockOCMClient.On("GetRegistryCredentials", mock.Anything, mock.Anything).Return(tc.registryCreds, tc.getRegCredsErr)
			mockOCMClient.On("Close", mock.Anything).Return(nil)

			// Create the validatePullSecretOptions object with mock dependencies
			options := &validatePullSecretOptions{
				clusterID:          "clusterID",
				pullSecretFetcher:  mockOCMClient,
				oCMClientInterface: mockOCMClient,
			}

			// Run the function
			_, err, _ := options.getPullSecretFromOCM()

			if err != nil {
				return
			}

		})
	}
}
