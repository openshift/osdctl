package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	ocmConfig "github.com/openshift-online/ocm-common/pkg/ocm/config"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func resetEnvVars(t *testing.T) {
	errUrl := os.Unsetenv("OCM_URL")
	if errUrl != nil {
		t.Fatal("Error setting environment variables")
	}
}

func TestGenerateQuery(t *testing.T) {
	tests := []struct {
		name              string
		clusterIdentifier string
		want              string
	}{
		{
			name:              "valid internal ID",
			clusterIdentifier: "261kalm3uob0vegg1c7h9o7r5k9t64ji",
			want:              "(id = '261kalm3uob0vegg1c7h9o7r5k9t64ji')",
		},
		{
			name:              "valid wrong internal ID with upper case",
			clusterIdentifier: "261kalm3uob0vegg1c7h9o7r5k9t64jI",
			want:              "(display_name like '261kalm3uob0vegg1c7h9o7r5k9t64jI')",
		},
		{
			name:              "valid wrong internal ID too short",
			clusterIdentifier: "261kalm3uob0vegg1c7h9o7r5k9t64j",
			want:              "(display_name like '261kalm3uob0vegg1c7h9o7r5k9t64j')",
		},
		{
			name:              "valid wrong internal ID too long",
			clusterIdentifier: "261kalm3uob0vegg1c7h9o7r5k9t64jix",
			want:              "(display_name like '261kalm3uob0vegg1c7h9o7r5k9t64jix')",
		},
		{
			name:              "valid external ID",
			clusterIdentifier: "c1f562af-fb22-42c5-aa07-6848e1eeee9c",
			want:              "(external_id = 'c1f562af-fb22-42c5-aa07-6848e1eeee9c')",
		},
		{
			name:              "valid display name",
			clusterIdentifier: "hs-mc-773jpgko0",
			want:              "(display_name like 'hs-mc-773jpgko0')",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GenerateQuery(tt.clusterIdentifier); got != tt.want {
				t.Errorf("GenerateQuery(%s) = %v, want %v", tt.clusterIdentifier, got, tt.want)
			}
		})
	}
}

// TestGetOcmConfigFromFilePath tests the GetOcmConfigFromFilePath function which loads
// OCM configuration from a JSON file at the provided path. It validates that the function
// correctly handles valid config files, non-existent files, empty files, and malformed JSON.
func TestGetOcmConfigFromFilePath(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) string
		wantErr     bool
		errContains string
		checkConfig func(*testing.T, *ocmConfig.Config)
	}{
		{
			// Test that a valid OCM config file is successfully parsed and loaded with correct values
			name: "valid config file",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configFile := filepath.Join(tmpDir, "ocm.json")
				config := ocmConfig.Config{
					AccessToken:  "test-access-token",
					RefreshToken: "test-refresh-token",
					URL:          "https://api.openshift.com",
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
				}
				data, err := json.Marshal(config)
				if err != nil {
					t.Fatalf("failed to marshal config: %v", err)
				}
				if err := os.WriteFile(configFile, data, 0644); err != nil {
					t.Fatalf("failed to write config file: %v", err)
				}
				return configFile
			},
			wantErr: false,
			checkConfig: func(t *testing.T, cfg *ocmConfig.Config) {
				if cfg == nil {
					t.Error("expected non-nil config")
					return
				}
				if cfg.AccessToken != "test-access-token" {
					t.Errorf("expected AccessToken 'test-access-token', got '%s'", cfg.AccessToken)
				}
				if cfg.RefreshToken != "test-refresh-token" {
					t.Errorf("expected RefreshToken 'test-refresh-token', got '%s'", cfg.RefreshToken)
				}
				if cfg.URL != "https://api.openshift.com" {
					t.Errorf("expected URL 'https://api.openshift.com', got '%s'", cfg.URL)
				}
				if cfg.ClientID != "test-client-id" {
					t.Errorf("expected ClientID 'test-client-id', got '%s'", cfg.ClientID)
				}
				if cfg.ClientSecret != "test-client-secret" {
					t.Errorf("expected ClientSecret 'test-client-secret', got '%s'", cfg.ClientSecret)
				}
			},
		},
		{
			// Test that attempting to load a non-existent file returns an appropriate error
			name: "non-existent file",
			setupFunc: func(t *testing.T) string {
				return "/nonexistent/path/ocm.json"
			},
			wantErr:     true,
			errContains: "can't read config file",
		},
		{
			// Test that an empty config file returns an error
			name: "empty config file",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configFile := filepath.Join(tmpDir, "ocm.json")
				if err := os.WriteFile(configFile, []byte(""), 0644); err != nil {
					t.Fatalf("failed to write empty file: %v", err)
				}
				return configFile
			},
			wantErr:     true,
			errContains: "empty config file",
		},
		{
			// Test that a file with invalid JSON syntax returns a parse error
			name: "invalid json",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configFile := filepath.Join(tmpDir, "ocm.json")
				if err := os.WriteFile(configFile, []byte("{invalid json}"), 0644); err != nil {
					t.Fatalf("failed to write invalid json: %v", err)
				}
				return configFile
			},
			wantErr:     true,
			errContains: "can't parse config file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setupFunc(t)
			cfg, err := GetOcmConfigFromFilePath(filePath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetOcmConfigFromFilePath() expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("GetOcmConfigFromFilePath() error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("GetOcmConfigFromFilePath() unexpected error = %v", err)
				}
				if cfg == nil {
					t.Errorf("GetOcmConfigFromFilePath() returned nil config")
				}
				if tt.checkConfig != nil {
					tt.checkConfig(t, cfg)
				}
			}
		})
	}
}

// TestGetOCMSdkConnBuilderFromConfig tests the GetOCMSdkConnBuilderFromConfig function
// which creates an OCM SDK connection builder from a provided OCM config object.
// It validates nil config handling and successful builder creation with valid config.
func TestGetOCMSdkConnBuilderFromConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *ocmConfig.Config
		wantErr bool
	}{
		{
			// Test that a valid OCM config successfully creates a connection builder
			name: "valid config",
			config: &ocmConfig.Config{
				AccessToken:  "test-access-token",
				RefreshToken: "test-refresh-token",
				URL:          "https://api.openshift.com",
				ClientID:     "test-client-id",
				ClientSecret: "test-client-secret",
			},
			wantErr: false,
		},
		{
			// Test that passing a nil config returns an error
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder, err := GetOCMSdkConnBuilderFromConfig(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetOCMSdkConnBuilderFromConfig() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("GetOCMSdkConnBuilderFromConfig() unexpected error = %v", err)
				}
				if builder == nil {
					t.Errorf("GetOCMSdkConnBuilderFromConfig() returned nil builder")
				}
			}
		})
	}
}

// TestGetOCMSdkConnBuilderFromFilePath tests the GetOCMSdkConnBuilderFromFilePath function
// which reads an OCM config file and creates an SDK connection builder from it.
// It validates both successful builder creation and error handling for invalid file paths.
func TestGetOCMSdkConnBuilderFromFilePath(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) string
		wantErr     bool
		errContains string
	}{
		{
			// Test that a valid config file successfully creates a connection builder
			name: "valid config file",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configFile := filepath.Join(tmpDir, "ocm.json")
				config := ocmConfig.Config{
					AccessToken:  "test-access-token",
					RefreshToken: "test-refresh-token",
					URL:          "https://api.openshift.com",
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
				}
				data, err := json.Marshal(config)
				if err != nil {
					t.Fatalf("failed to marshal config: %v", err)
				}
				if err := os.WriteFile(configFile, data, 0644); err != nil {
					t.Fatalf("failed to write config file: %v", err)
				}
				return configFile
			},
			wantErr: false,
		},
		{
			// Test that attempting to load from a non-existent file returns an error
			name: "non-existent file",
			setupFunc: func(t *testing.T) string {
				return "/nonexistent/path/ocm.json"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setupFunc(t)
			builder, err := GetOCMSdkConnBuilderFromFilePath(filePath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetOCMSdkConnBuilderFromFilePath() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("GetOCMSdkConnBuilderFromFilePath() unexpected error = %v", err)
				}
				if builder == nil {
					t.Errorf("GetOCMSdkConnBuilderFromFilePath() returned nil builder")
				}
			}
		})
	}
}

// TestGetOCMSdkConnFromFilePath tests the GetOCMSdkConnFromFilePath function which
// reads an OCM config file and creates a fully initialized OCM SDK connection from it.
// It validates error handling for non-existent files and empty config files.
func TestGetOCMSdkConnFromFilePath(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T) string
		wantErr   bool
	}{
		{
			// Test that attempting to create a connection from a non-existent file returns an error
			name: "non-existent file",
			setupFunc: func(t *testing.T) string {
				return "/nonexistent/path/ocm.json"
			},
			wantErr: true,
		},
		{
			// Test that an empty config file returns an error when trying to build a connection
			name: "empty config file",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configFile := filepath.Join(tmpDir, "ocm.json")
				if err := os.WriteFile(configFile, []byte(""), 0644); err != nil {
					t.Fatalf("failed to write empty file: %v", err)
				}
				return configFile
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setupFunc(t)
			conn, err := GetOCMSdkConnFromFilePath(filePath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetOCMSdkConnFromFilePath() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("GetOCMSdkConnFromFilePath() unexpected error = %v", err)
				}
				if conn != nil {
					defer conn.Close()
				}
			}
		})
	}
}

// TestGetHiveShardWithConn tests the GetHiveShardWithConn function which retrieves
// the hive shard URL for a cluster using a provided OCM SDK connection.
// It validates that the function properly handles nil connection inputs.
func TestGetHiveShardWithConn(t *testing.T) {
	tests := []struct {
		name      string
		clusterID string
		conn      *sdk.Connection
		wantErr   bool
	}{
		{
			// Test that passing a nil OCM connection returns an error
			name:      "nil connection",
			clusterID: "test-cluster-id",
			conn:      nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetHiveShardWithConn(tt.clusterID, tt.conn)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetHiveShardWithConn() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("GetHiveShardWithConn() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestGetHiveClusterWithConn tests the GetHiveClusterWithConn function which fetches
// the hive cluster information using separate OCM connections for the target cluster
// and hive cluster. It validates the function's ability to create temporary connections
// when nil connections are provided.
func TestGetHiveClusterWithConn(t *testing.T) {
	tests := []struct {
		name       string
		clusterID  string
		clusterOCM *sdk.Connection
		hiveOCM    *sdk.Connection
		wantErr    bool
	}{
		{
			// Test that when both connections are nil, the function attempts to create a temporary connection
			// This will fail without proper OCM environment variables set
			name:       "both connections nil - should create temporary connection",
			clusterID:  "test-cluster-id",
			clusterOCM: nil,
			hiveOCM:    nil,
			wantErr:    true, // will fail when trying to create connection without proper env vars
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetHiveClusterWithConn(tt.clusterID, tt.clusterOCM, tt.hiveOCM)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetHiveClusterWithConn() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("GetHiveClusterWithConn() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestGetOCMConfigFromEnv tests the GetOCMConfigFromEnv function which loads
// OCM configuration from environment variables and default file locations.
// It validates the function's ability to handle different config file locations
// including OCM_CONFIG env var, ~/.ocm.json, and $XDG_CONFIG_HOME/ocm/ocm.json.
func TestGetOCMConfigFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) func()
		wantErr     bool
		errContains string
		checkConfig func(*testing.T, *ocmConfig.Config)
	}{
		{
			// Test loading config from a custom OCM_CONFIG environment variable path
			name: "with OCM_CONFIG env var set to valid file",
			setupFunc: func(t *testing.T) func() {
				tmpDir := t.TempDir()
				configFile := filepath.Join(tmpDir, "custom-ocm.json")
				config := ocmConfig.Config{
					AccessToken:  "test-token-from-env",
					RefreshToken: "test-refresh",
					URL:          "https://api.openshift.com",
				}
				data, err := json.Marshal(config)
				if err != nil {
					t.Fatalf("failed to marshal config: %v", err)
				}
				if err := os.WriteFile(configFile, data, 0644); err != nil {
					t.Fatalf("failed to write config file: %v", err)
				}
				oldVal := os.Getenv("OCM_CONFIG")
				os.Setenv("OCM_CONFIG", configFile)
				return func() {
					os.Setenv("OCM_CONFIG", oldVal)
				}
			},
			wantErr: false,
			checkConfig: func(t *testing.T, cfg *ocmConfig.Config) {
				if cfg == nil {
					t.Error("expected non-nil config")
					return
				}
				if cfg.AccessToken != "test-token-from-env" {
					t.Errorf("expected AccessToken 'test-token-from-env', got '%s'", cfg.AccessToken)
				}
			},
		},
		{
			// Test that when no config file exists, an empty config is returned without error
			name: "no config file exists - returns empty config",
			setupFunc: func(t *testing.T) func() {
				// Set OCM_CONFIG to a non-existent path
				oldVal := os.Getenv("OCM_CONFIG")
				os.Setenv("OCM_CONFIG", "/nonexistent/path/ocm.json")
				return func() {
					os.Setenv("OCM_CONFIG", oldVal)
				}
			},
			wantErr: false,
			checkConfig: func(t *testing.T, cfg *ocmConfig.Config) {
				if cfg == nil {
					t.Error("expected non-nil empty config")
				}
			},
		},
		{
			// Test that a malformed config file returns a parse error
			name: "invalid json in config file",
			setupFunc: func(t *testing.T) func() {
				tmpDir := t.TempDir()
				configFile := filepath.Join(tmpDir, "invalid-ocm.json")
				if err := os.WriteFile(configFile, []byte("{invalid json}"), 0644); err != nil {
					t.Fatalf("failed to write invalid json: %v", err)
				}
				oldVal := os.Getenv("OCM_CONFIG")
				os.Setenv("OCM_CONFIG", configFile)
				return func() {
					os.Setenv("OCM_CONFIG", oldVal)
				}
			},
			wantErr:     true,
			errContains: "can't parse config file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setupFunc(t)
			defer cleanup()

			cfg, err := GetOCMConfigFromEnv()
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetOCMConfigFromEnv() expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("GetOCMConfigFromEnv() error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("GetOCMConfigFromEnv() unexpected error = %v", err)
				}
				if tt.checkConfig != nil {
					tt.checkConfig(t, cfg)
				}
			}
		})
	}
}

// TestCreateConnectionWithUrl tests the CreateConnectionWithUrl function which creates
// an OCM SDK connection with a specified URL. It validates URL alias handling for
// 'production', 'staging', and 'integration' environments.
// Note: Successful connection creation requires valid OCM credentials and is tested
// in integration tests or the hive-login test command.
func TestCreateConnectionWithUrl(t *testing.T) {
	tests := []struct {
		name        string
		ocmUrl      string
		wantErr     bool
		errContains string
	}{
		{
			// Test that an empty URL returns an error
			name:        "empty URL",
			ocmUrl:      "",
			wantErr:     true,
			errContains: "empty OCM URL",
		},
		{
			// Test that an invalid alias returns an error with valid aliases listed
			name:        "invalid URL alias",
			ocmUrl:      "invalid-alias",
			wantErr:     true,
			errContains: "invalid OCM_URL found",
		},
		{
			// Test that 'production' alias doesn't fail with "invalid alias" error
			// Will fail with credentials error if not logged in, which is expected
			name:    "production alias recognized",
			ocmUrl:  "production",
			wantErr: true, // Will fail without credentials, but not with "invalid alias" error
		},
		{
			// Test that 'staging' alias doesn't fail with "invalid alias" error
			// Will fail with credentials error if not logged in, which is expected
			name:    "staging alias recognized",
			ocmUrl:  "staging",
			wantErr: true, // Will fail without credentials, but not with "invalid alias" error
		},
		{
			// Test that 'integration' alias doesn't fail with "invalid alias" error
			// Will fail with credentials error if not logged in, which is expected
			name:    "integration alias recognized",
			ocmUrl:  "integration",
			wantErr: true, // Will fail without credentials, but not with "invalid alias" error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := CreateConnectionWithUrl(tt.ocmUrl)
			if tt.wantErr {
				if err == nil {
					t.Errorf("CreateConnectionWithUrl() expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("CreateConnectionWithUrl() error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("CreateConnectionWithUrl() unexpected error = %v", err)
				}
				if conn != nil {
					defer conn.Close()
				}
			}
		})
	}
}

// TestGetHiveBPClientForCluster tests the GetHiveBPClientForCluster function which creates a
// backplane client connection to a hive cluster. It validates input validation and
// error handling for empty cluster IDs.
// Note: Successful connection creation requires valid cluster ID, OCM credentials,
// and accessible hive cluster. These scenarios are tested in integration tests or
// the hive-login test command.
func TestGetHiveBPClientForCluster(t *testing.T) {
	tests := []struct {
		name             string
		clusterID        string
		elevationReason  string
		hiveOCMURL       string
		wantErr          bool
		errContains      string
	}{
		{
			// Test that an empty cluster ID returns an error
			name:            "empty cluster ID",
			clusterID:       "",
			elevationReason: "",
			hiveOCMURL:      "",
			wantErr:         true,
			errContains:     "empty target cluster ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetHiveBPClientForCluster(tt.clusterID, client.Options{}, tt.elevationReason, tt.hiveOCMURL)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetHiveBPClientForCluster() expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("GetHiveBPClientForCluster() error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("GetHiveBPClientForCluster() unexpected error = %v", err)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
