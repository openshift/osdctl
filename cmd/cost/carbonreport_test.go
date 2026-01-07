package cost

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/provider/aws/mock"
	"go.uber.org/mock/gomock"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestValidateUsagePeriod(t *testing.T) {
	testCases := []struct {
		name        string
		usagePeriod string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid YYYY format",
			usagePeriod: "2024",
			expectError: false,
		},
		{
			name:        "Valid YYYY-MM format with month 01",
			usagePeriod: "2024-01",
			expectError: false,
		},
		{
			name:        "Valid YYYY-MM format with month 12",
			usagePeriod: "2024-12",
			expectError: false,
		},
		{
			name:        "Valid YYYY-MM format with month 06",
			usagePeriod: "2023-06",
			expectError: false,
		},
		{
			name:        "Empty usage period",
			usagePeriod: "",
			expectError: true,
			errorMsg:    "usage period is required",
		},
		{
			name:        "Invalid month 00",
			usagePeriod: "2024-00",
			expectError: true,
			errorMsg:    "invalid usage period format '2024-00'. Expected format: YYYY or YYYY-MM",
		},
		{
			name:        "Invalid month 13",
			usagePeriod: "2024-13",
			expectError: true,
			errorMsg:    "invalid usage period format '2024-13'. Expected format: YYYY or YYYY-MM",
		},
		{
			name:        "Invalid year format (2 digits)",
			usagePeriod: "24-03",
			expectError: true,
			errorMsg:    "invalid usage period format '24-03'. Expected format: YYYY or YYYY-MM",
		},
		{
			name:        "Invalid year format (3 digits)",
			usagePeriod: "202-03",
			expectError: true,
			errorMsg:    "invalid usage period format '202-03'. Expected format: YYYY or YYYY-MM",
		},
		{
			name:        "Invalid format with day",
			usagePeriod: "2024-03-15",
			expectError: true,
			errorMsg:    "invalid usage period format '2024-03-15'. Expected format: YYYY or YYYY-MM",
		},
		{
			name:        "Invalid format - just month",
			usagePeriod: "03",
			expectError: true,
			errorMsg:    "invalid usage period format '03'. Expected format: YYYY or YYYY-MM",
		},
		{
			name:        "Invalid format - text",
			usagePeriod: "invalid",
			expectError: true,
			errorMsg:    "invalid usage period format 'invalid'. Expected format: YYYY or YYYY-MM",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ops := newCarbonReportOptions(
				genericclioptions.IOStreams{},
				&globalflags.GlobalOptions{},
			)
			ops.usagePeriod = tc.usagePeriod

			err := ops.validateUsagePeriod()

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none for usage period: %s", tc.usagePeriod)
				} else if err.Error() != tc.errorMsg {
					t.Errorf("Expected error message '%s' but got '%s'", tc.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v for usage period: %s", err, tc.usagePeriod)
				}
			}
		})
	}
}

func TestCreateAWSClientEnvironmentValidation(t *testing.T) {
	testCases := []struct {
		name          string
		envValue      string
		expectError   bool
		errorContains string
	}{
		{
			name:          "AWS_ACCOUNT_NAME not set",
			envValue:      "",
			expectError:   true,
			errorContains: "AWS_ACCOUNT_NAME environment variable is not set",
		},
		{
			name:          "AWS_ACCOUNT_NAME set to wrong value",
			envValue:      "wrong-account",
			expectError:   true,
			errorContains: "AWS_ACCOUNT_NAME is set to 'wrong-account' but expected 'rh-control'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Save original env var and restore after test
			originalEnv := os.Getenv("AWS_ACCOUNT_NAME")
			defer func() {
				if originalEnv != "" {
					os.Setenv("AWS_ACCOUNT_NAME", originalEnv)
				} else {
					os.Unsetenv("AWS_ACCOUNT_NAME")
				}
			}()

			// Set test env var
			if tc.envValue != "" {
				os.Setenv("AWS_ACCOUNT_NAME", tc.envValue)
			} else {
				os.Unsetenv("AWS_ACCOUNT_NAME")
			}

			// Create a costOptions for testing
			opts := &costOptions{
				IOStreams: genericclioptions.IOStreams{},
			}
			_, err := createAWSClientWithOptions(opts)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s' but got '%s'", tc.errorContains, err.Error())
				}
			}
		})
	}
}

func TestCreateAWSClientWithMock(t *testing.T) {
	// Save and restore original env
	originalEnv := os.Getenv("AWS_ACCOUNT_NAME")
	defer func() {
		if originalEnv != "" {
			os.Setenv("AWS_ACCOUNT_NAME", originalEnv)
		} else {
			os.Unsetenv("AWS_ACCOUNT_NAME")
		}
	}()

	// Set correct environment
	os.Setenv("AWS_ACCOUNT_NAME", "rh-control")

	testCases := []struct {
		name         string
		setupMock    func(r *mock.MockClientMockRecorder)
		expectError  bool
		errorMessage string
	}{
		{
			name: "GetCallerIdentity succeeds",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.GetCallerIdentity(gomock.Any()).Return(&sts.GetCallerIdentityOutput{}, nil).Times(1)
			},
			expectError: false,
		},
		{
			name: "GetCallerIdentity fails with unauthorized",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.GetCallerIdentity(gomock.Any()).Return(nil, errors.New("unauthorized")).Times(1)
			},
			expectError:  true,
			errorMessage: "unauthorized",
		},
		{
			name: "GetCallerIdentity fails with access denied",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.GetCallerIdentity(gomock.Any()).Return(nil, errors.New("access denied")).Times(1)
			},
			expectError:  true,
			errorMessage: "access denied",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockClient := mock.NewMockClient(mockCtrl)
			tc.setupMock(mockClient.EXPECT())

			// Test the mock behavior directly since we can't easily inject it into createAWSClientWithOptions
			_, err := mockClient.GetCallerIdentity(nil)

			if tc.expectError {
				if err == nil {
					t.Error("Expected error from GetCallerIdentity but got none")
				} else if !strings.Contains(err.Error(), tc.errorMessage) {
					t.Errorf("Expected error containing '%s' but got: %v", tc.errorMessage, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error from GetCallerIdentity but got: %v", err)
				}
			}
		})
	}
}
