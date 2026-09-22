package rhobs

import (
	"testing"
)

func TestGetTokenProvider_CredentialResolution(t *testing.T) {
	// Save original commonOptions
	origCommonOptions := commonOptions
	defer func() {
		commonOptions = origCommonOptions
	}()

	tests := []struct {
		name             string
		flagClientID     string
		flagClientSecret string
		envClientID      string
		envClientSecret  string
		wantErr          bool
		errContains      string
	}{
		{
			name:             "both flags provided - should succeed",
			flagClientID:     "test-id",
			flagClientSecret: "test-secret",
			wantErr:          false,
		},
		{
			name:             "both env vars provided - should succeed",
			envClientID:      "test-id",
			envClientSecret:  "test-secret",
			wantErr:          false,
		},
		{
			name:             "flags override env vars - should succeed",
			flagClientID:     "flag-id",
			flagClientSecret: "flag-secret",
			envClientID:      "env-id",
			envClientSecret:  "env-secret",
			wantErr:          false,
		},
		{
			name:         "only flag client ID - should error",
			flagClientID: "test-id",
			wantErr:      true,
			errContains:  "must be provided together",
		},
		{
			name:             "only flag client secret - should error",
			flagClientSecret: "test-secret",
			wantErr:          true,
			errContains:      "must be provided together",
		},
		{
			name:        "only env client ID - should error",
			envClientID: "test-id",
			wantErr:     true,
			errContains: "must be provided together",
		},
		{
			name:            "only env client secret - should error",
			envClientSecret: "test-secret",
			wantErr:         true,
			errContains:     "must be provided together",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset commonOptions
			commonOptions.clientID = ""
			commonOptions.clientSecret = ""

			// Set up test inputs - t.Setenv automatically restores after subtest
			// Set both unconditionally to override any pre-existing CI environment
			t.Setenv(rhobsClientIDEnvVar, tt.envClientID)
			t.Setenv(rhobsClientSecretEnvVar, tt.envClientSecret)
			commonOptions.clientID = tt.flagClientID
			commonOptions.clientSecret = tt.flagClientSecret

			// Create fetcher with minimal required fields
			fetcher := &RhobsFetcher{
				ocmEnvName: "production",
			}

			// Test getTokenProvider
			provider, err := fetcher.getTokenProvider()

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if provider == nil {
					t.Error("expected non-nil provider, got nil")
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && someContains(s, substr))
}

func someContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
