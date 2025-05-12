package account

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestPrependRegionToURL(t *testing.T) {
	tests := []struct {
		name        string
		consoleURL  string
		region      string
		expectedURL string
		expectErr   bool
	}{
		{
			name:        "valid_url_with_destination",
			consoleURL:  "https://example.com/login?Destination=https%3A%2F%2Fservice.com%2Fhome",
			region:      "us-west-2",
			expectedURL: "https://example.com/login?Destination=https%3A%2F%2Fus-west-2.service.com%2Fhome",
			expectErr:   false,
		},
		{
			name:        "invalid_console_url",
			consoleURL:  "http://[::1]:namedport",
			region:      "us-west-2",
			expectedURL: "",
			expectErr:   true,
		},
		{
			name:        "missing_destination",
			consoleURL:  "https://example.com/login",
			region:      "us-west-2",
			expectedURL: "https://example.com/login?Destination=%2F%2Fus-west-2.",
			expectErr:   false,
		},
		{
			name:        "invalid_destination_url",
			consoleURL:  "https://example.com/login?Destination=::badurl::",
			region:      "us-west-2",
			expectedURL: "",
			expectErr:   true,
		},
		{
			name:        "valid_destination_with_query",
			consoleURL:  "https://example.com/login?Destination=https%3A%2F%2Fservice.com%2Fhome%3Ffoo%3Dbar",
			region:      "ap-southeast-1",
			expectedURL: "https://example.com/login?Destination=https%3A%2F%2Fap-southeast-1.service.com%2Fhome%3Ffoo%3Dbar",
			expectErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := PrependRegionToURL(tt.consoleURL, tt.region)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedURL, result)
			}
		})
	}
}

func TestNewCmdConsole(t *testing.T) {
	tests := []struct {
		name           string
		flags          map[string]string
		expectedErr    bool
		expectedRegion string
	}{
		{
			name:        "missing_account_id_flag",
			flags:       map[string]string{},
			expectedErr: true,
		},
		{
			name: "valid_input_with_region",
			flags: map[string]string{
				"accountId": "123456789012",
				"region":    "ap-south-1",
			},
			expectedErr:    false,
			expectedRegion: "ap-south-1",
		},
		{
			name: "valid_input_without_region",
			flags: map[string]string{
				"accountId": "123456789012",
			},
			expectedErr:    false,
			expectedRegion: "us-east-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := newConsoleOptions()

			// Simulate cobra.Command context
			cmd := &cobra.Command{}
			cmd.Flags().StringVarP(&ops.awsAccountID, "accountId", "i", "", "")
			cmd.Flags().StringVarP(&ops.region, "region", "r", "", "")

			// Set flags
			for k, v := range tt.flags {
				_ = cmd.Flags().Set(k, v)
			}

			// Manually call ops.complete
			err := ops.complete(cmd)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedRegion, ops.region)
			}
		})
	}
}
