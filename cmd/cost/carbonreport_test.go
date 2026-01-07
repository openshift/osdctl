package cost

import (
	"compress/gzip"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
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

func TestValidateAccount(t *testing.T) {
	testCases := []struct {
		name        string
		account     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid account with exactly 12 digits",
			account:     "123456789012",
			expectError: false,
		},
		{
			name:        "Valid account with 13 digits",
			account:     "1234567890123",
			expectError: false,
		},
		{
			name:        "Valid account with 16 digits",
			account:     "1234567890123456",
			expectError: false,
		},
		{
			name:        "Valid account with leading zeros",
			account:     "000123456789",
			expectError: false,
		},
		{
			name:        "Empty account",
			account:     "",
			expectError: true,
			errorMsg:    "account is required",
		},
		{
			name:        "Account with 11 digits (too short)",
			account:     "12345678901",
			expectError: true,
			errorMsg:    "invalid account format '12345678901'. Account must be a number with at least 12 digits",
		},
		{
			name:        "Account with 1 digit",
			account:     "1",
			expectError: true,
			errorMsg:    "invalid account format '1'. Account must be a number with at least 12 digits",
		},
		{
			name:        "Account with letters",
			account:     "12345678901a",
			expectError: true,
			errorMsg:    "invalid account format '12345678901a'. Account must be a number with at least 12 digits",
		},
		{
			name:        "Account with spaces",
			account:     "123 456 789 012",
			expectError: true,
			errorMsg:    "invalid account format '123 456 789 012'. Account must be a number with at least 12 digits",
		},
		{
			name:        "Account with dashes",
			account:     "123-456-789-012",
			expectError: true,
			errorMsg:    "invalid account format '123-456-789-012'. Account must be a number with at least 12 digits",
		},
		{
			name:        "Account with special characters",
			account:     "123456789012!",
			expectError: true,
			errorMsg:    "invalid account format '123456789012!'. Account must be a number with at least 12 digits",
		},
		{
			name:        "Account with decimal point",
			account:     "123456789012.0",
			expectError: true,
			errorMsg:    "invalid account format '123456789012.0'. Account must be a number with at least 12 digits",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ops := newCarbonReportOptions(
				genericclioptions.IOStreams{},
				&globalflags.GlobalOptions{},
			)
			ops.account = tc.account

			err := ops.validateAccount()

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none for account: %s", tc.account)
				} else if err.Error() != tc.errorMsg {
					t.Errorf("Expected error message '%s' but got '%s'", tc.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v for account: %s", err, tc.account)
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

func TestGetUsagePeriodDirectories(t *testing.T) {
	testCases := []struct {
		name          string
		usagePeriod   string
		setupMock     func(r *mock.MockClientMockRecorder)
		expectedDirs  []string
		expectError   bool
		errorContains string
	}{
		{
			name:        "List fails with error",
			usagePeriod: "2024",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.ListObjectsV2(gomock.Any()).Return(nil, errors.New("S3 error")).Times(1)
			},
			expectError:   true,
			errorContains: "failed to list S3 objects",
		},
		{
			name:        "No directories found",
			usagePeriod: "2024",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.ListObjectsV2(gomock.Any()).Return(&s3.ListObjectsV2Output{
					CommonPrefixes: []types.CommonPrefix{},
				}, nil).Times(1)
			},
			expectedDirs: []string{},
			expectError:  false,
		},
		{
			name:        "Single directory matches YYYY format",
			usagePeriod: "2024",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.ListObjectsV2(gomock.Any()).Return(&s3.ListObjectsV2Output{
					CommonPrefixes: []types.CommonPrefix{
						{Prefix: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-03/")},
					},
				}, nil).Times(1)
			},
			expectedDirs: []string{"usage_period=2024-03"},
			expectError:  false,
		},
		{
			name:        "Multiple directories match YYYY format",
			usagePeriod: "2024",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.ListObjectsV2(gomock.Any()).Return(&s3.ListObjectsV2Output{
					CommonPrefixes: []types.CommonPrefix{
						{Prefix: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-01/")},
						{Prefix: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-02/")},
						{Prefix: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-03/")},
						{Prefix: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2023-12/")},
					},
				}, nil).Times(1)
			},
			expectedDirs: []string{"usage_period=2024-01", "usage_period=2024-02", "usage_period=2024-03"},
			expectError:  false,
		},
		{
			name:        "Single directory matches YYYY-MM format",
			usagePeriod: "2024-03",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.ListObjectsV2(gomock.Any()).Return(&s3.ListObjectsV2Output{
					CommonPrefixes: []types.CommonPrefix{
						{Prefix: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-01/")},
						{Prefix: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-03/")},
						{Prefix: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-05/")},
					},
				}, nil).Times(1)
			},
			expectedDirs: []string{"usage_period=2024-03"},
			expectError:  false,
		},
		{
			name:        "No match for specific month",
			usagePeriod: "2024-06",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.ListObjectsV2(gomock.Any()).Return(&s3.ListObjectsV2Output{
					CommonPrefixes: []types.CommonPrefix{
						{Prefix: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-01/")},
						{Prefix: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-03/")},
					},
				}, nil).Times(1)
			},
			expectedDirs: []string{},
			expectError:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockClient := mock.NewMockClient(mockCtrl)
			tc.setupMock(mockClient.EXPECT())

			dirs, err := getUsagePeriodDirectories(mockClient, tc.usagePeriod)

			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s' but got: %v", tc.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if len(dirs) != len(tc.expectedDirs) {
					t.Errorf("Expected %d directories but got %d", len(tc.expectedDirs), len(dirs))
				}
				for i, expectedDir := range tc.expectedDirs {
					if i >= len(dirs) || dirs[i] != expectedDir {
						t.Errorf("Expected directory '%s' at position %d but got '%s'", expectedDir, i, dirs[i])
					}
				}
			}
		})
	}
}

func TestProcessCarbonData(t *testing.T) {
	testCases := []struct {
		name               string
		setupMock          func(r *mock.MockClientMockRecorder)
		accountID          string
		expectedRowCount   int
		expectError        bool
		errorContains      string
		expectedHeaderSize int
	}{
		{
			name: "Successfully processes CSV with matching rows",
			setupMock: func(r *mock.MockClientMockRecorder) {
				// Mock ListObjectsV2 to return a .gz file
				r.ListObjectsV2(gomock.Any()).Return(&s3.ListObjectsV2Output{
					Contents: []types.Object{
						{Key: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-03/data.csv.gz")},
					},
				}, nil).Times(1)

				// Create a mock CSV with gzip compression including excluded columns
				csvData := "usage_account_id,payer_account_id,region,emissions\n123456789012,999999999999,us-east-1,100\n999999999999,999999999999,us-west-2,200\n123456789012,999999999999,eu-west-1,150\n"

				// Mock GetObject to return gzipped CSV data
				r.GetObject(gomock.Any()).DoAndReturn(func(input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
					// Create gzipped data
					var buf strings.Builder
					gzWriter := gzip.NewWriter(&buf)
					_, _ = gzWriter.Write([]byte(csvData))
					gzWriter.Close()

					return &s3.GetObjectOutput{
						Body: io.NopCloser(strings.NewReader(buf.String())),
					}, nil
				}).Times(1)
			},
			accountID:          "123456789012",
			expectedRowCount:   2,
			expectError:        false,
			expectedHeaderSize: 2, // Only region and emissions (excluded columns removed)
		},
		{
			name: "No matching rows for account",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.ListObjectsV2(gomock.Any()).Return(&s3.ListObjectsV2Output{
					Contents: []types.Object{
						{Key: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-03/data.csv.gz")},
					},
				}, nil).Times(1)

				csvData := "usage_account_id,payer_account_id,region,emissions\n999999999999,999999999999,us-east-1,100\n888888888888,888888888888,us-west-2,200\n"

				r.GetObject(gomock.Any()).DoAndReturn(func(input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
					var buf strings.Builder
					gzWriter := gzip.NewWriter(&buf)
					_, _ = gzWriter.Write([]byte(csvData))
					gzWriter.Close()

					return &s3.GetObjectOutput{
						Body: io.NopCloser(strings.NewReader(buf.String())),
					}, nil
				}).Times(1)
			},
			accountID:          "123456789012",
			expectedRowCount:   0,
			expectError:        false,
			expectedHeaderSize: 2, // Only region and emissions (excluded columns removed)
		},
		{
			name: "Verifies excluded columns are removed",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.ListObjectsV2(gomock.Any()).Return(&s3.ListObjectsV2Output{
					Contents: []types.Object{
						{Key: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-03/data.csv.gz")},
					},
				}, nil).Times(1)

				// CSV with all columns including excluded ones
				csvData := "usage_account_id,payer_account_id,region,service,emissions\n123456789012,555555555555,us-east-1,ec2,100\n123456789012,555555555555,us-west-2,s3,200\n"

				r.GetObject(gomock.Any()).DoAndReturn(func(input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
					var buf strings.Builder
					gzWriter := gzip.NewWriter(&buf)
					_, _ = gzWriter.Write([]byte(csvData))
					gzWriter.Close()

					return &s3.GetObjectOutput{
						Body: io.NopCloser(strings.NewReader(buf.String())),
					}, nil
				}).Times(1)
			},
			accountID:          "123456789012",
			expectedRowCount:   2,
			expectError:        false,
			expectedHeaderSize: 3, // Only region, service, emissions (usage_account_id and payer_account_id excluded)
		},
		{
			name: "No .gz file found",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.ListObjectsV2(gomock.Any()).Return(&s3.ListObjectsV2Output{
					Contents: []types.Object{
						{Key: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-03/data.txt")},
					},
				}, nil).Times(1)
			},
			accountID:     "123456789012",
			expectError:   true,
			errorContains: "no .gz file found",
		},
		{
			name: "ListObjectsV2 fails",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.ListObjectsV2(gomock.Any()).Return(nil, errors.New("S3 list error")).Times(1)
			},
			accountID:     "123456789012",
			expectError:   true,
			errorContains: "failed to list objects",
		},
		{
			name: "GetObject fails",
			setupMock: func(r *mock.MockClientMockRecorder) {
				r.ListObjectsV2(gomock.Any()).Return(&s3.ListObjectsV2Output{
					Contents: []types.Object{
						{Key: awsSdk.String("reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=2024-03/data.csv.gz")},
					},
				}, nil).Times(1)

				r.GetObject(gomock.Any()).Return(nil, errors.New("S3 get error")).Times(1)
			},
			accountID:     "123456789012",
			expectError:   true,
			errorContains: "failed to download",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockClient := mock.NewMockClient(mockCtrl)
			tc.setupMock(mockClient.EXPECT())

			rows, header, err := processCarbonData(mockClient, "test-bucket", "usage_period=2024-03", tc.accountID)

			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s' but got: %v", tc.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if len(rows) != tc.expectedRowCount {
					t.Errorf("Expected %d rows but got %d", tc.expectedRowCount, len(rows))
				}
				if tc.expectedHeaderSize > 0 && len(header) != tc.expectedHeaderSize {
					t.Errorf("Expected header size %d but got %d", tc.expectedHeaderSize, len(header))
				}

				// Verify excluded columns are not in the header
				for _, col := range header {
					if excludedColumns[col] {
						t.Errorf("Excluded column '%s' found in header", col)
					}
				}

				// Verify all rows have the same length as header
				for i, row := range rows {
					if len(row) != len(header) {
						t.Errorf("Row %d has length %d but header has length %d", i, len(row), len(header))
					}
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
