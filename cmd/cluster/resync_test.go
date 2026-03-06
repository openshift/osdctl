package cluster

import (
	"strings"
	"testing"

	"github.com/openshift/osdctl/pkg/utils"
)

// TestHiveOcmUrlValidation tests the early validation of --hive-ocm-url flag in the resync command
func TestHiveOcmUrlValidation(t *testing.T) {
	tests := []struct {
		name        string
		hiveOcmUrl  string
		expectErr   bool
		errContains string
	}{
		{
			name:       "Valid hive-ocm-url (production)",
			hiveOcmUrl: "production",
			expectErr:  false,
		},
		{
			name:       "Valid hive-ocm-url (staging)",
			hiveOcmUrl: "staging",
			expectErr:  false,
		},
		{
			name:       "Valid hive-ocm-url (integration)",
			hiveOcmUrl: "integration",
			expectErr:  false,
		},
		{
			name:       "Valid hive-ocm-url (full URL)",
			hiveOcmUrl: "https://api.openshift.com",
			expectErr:  false,
		},
		{
			name:        "Invalid hive-ocm-url",
			hiveOcmUrl:  "invalid-environment",
			expectErr:   true,
			errContains: "invalid OCM_URL",
		},
		{
			name:        "Empty hive-ocm-url",
			hiveOcmUrl:  "",
			expectErr:   true,
			errContains: "empty OCM URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This simulates the validation that occurs in the Resync.New() method
			var err error
			if tt.hiveOcmUrl != "" {
				_, err = utils.ValidateAndResolveOcmUrl(tt.hiveOcmUrl)
			} else {
				_, err = utils.ValidateAndResolveOcmUrl(tt.hiveOcmUrl)
			}

			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got nil", tt.errContains)
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			}
		})
	}
}
