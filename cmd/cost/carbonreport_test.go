package cost

import (
	"testing"

	"github.com/openshift/osdctl/internal/utils/globalflags"
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
