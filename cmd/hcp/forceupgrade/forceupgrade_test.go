package forceupgrade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestForceUpgradeOptionsValidation(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test-template.json")
	err := os.WriteFile(tmpFile, []byte(`{"test": "template"}`), 0600)
	if err != nil {
		t.Fatalf("Failed to create temp file for test: %v", err)
	}

	tests := []struct {
		name              string
		opts              *forceUpgradeOptions
		needsClustersFile bool
		wantErr           bool
		errMsg            string
	}{
		{
			name: "valid with cluster ID",
			opts: &forceUpgradeOptions{
				clusterID:      "test-cluster",
				nextRunMinutes: 10,
			},
			wantErr: false,
		},
		{
			name: "invalid - no cluster targeting",
			opts: &forceUpgradeOptions{
				nextRunMinutes: 10,
			},
			wantErr: true,
			errMsg:  "no cluster identifier has been found, please specify either --cluster-id or --clusters-file",
		},
		{
			name: "invalid - both cluster ID and file",
			opts: &forceUpgradeOptions{
				clusterID:      "test-cluster",
				clustersFile:   "clusters.json",
				nextRunMinutes: 10,
			},
			wantErr: true,
			errMsg:  "cannot specify both --cluster-id and --clusters-file, choose one",
		},
		{
			name: "invalid - next run minutes too low",
			opts: &forceUpgradeOptions{
				clusterID:      "test-cluster",
				nextRunMinutes: 5,
			},
			wantErr: true,
			errMsg:  "next-run-minutes must be at least 6 minutes",
		},
		{
			name: "valid - service log with template name",
			opts: &forceUpgradeOptions{
				clusterID:          "test-cluster",
				nextRunMinutes:     10,
				serviceLogTemplate: "end-of-support",
			},
			wantErr: false,
		},
		{
			name: "valid - service log with file path",
			opts: &forceUpgradeOptions{
				clusterID:          "test-cluster",
				nextRunMinutes:     10,
				serviceLogTemplate: tmpFile,
			},
			wantErr: false,
		},
		{
			name: "invalid - service log with invalid template name",
			opts: &forceUpgradeOptions{
				clusterID:          "test-cluster",
				nextRunMinutes:     10,
				serviceLogTemplate: "invalid-template",
			},
			wantErr: true,
			errMsg:  "service log value 'invalid-template' is neither a valid template name [end-of-support] nor an existing file",
		},
		{
			name: "invalid - service log file does not exist",
			opts: &forceUpgradeOptions{
				clusterID:          "test-cluster",
				nextRunMinutes:     10,
				serviceLogTemplate: "/nonexistent/file.json",
			},
			wantErr: true,
			errMsg:  "service log value '/nonexistent/file.json' is neither a valid template name [end-of-support] nor an existing file",
		},
		{
			name: "valid - cluster ID with alphanumeric and hyphens",
			opts: &forceUpgradeOptions{
				clusterID:      "test-cluster-123",
				nextRunMinutes: 10,
			},
			wantErr: false,
		},
		{
			name: "invalid - cluster ID with special characters",
			opts: &forceUpgradeOptions{
				clusterID:      "test@cluster!",
				nextRunMinutes: 10,
			},
			wantErr: true,
			errMsg:  "cluster ID 'test@cluster!' contains invalid characters - only alphanumeric characters and hyphens are allowed",
		},
		{
			name: "invalid - cluster ID with underscore",
			opts: &forceUpgradeOptions{
				clusterID:      "test_cluster",
				nextRunMinutes: 10,
			},
			wantErr: true,
			errMsg:  "cluster ID 'test_cluster' contains invalid characters - only alphanumeric characters and hyphens are allowed",
		},
		{
			name: "invalid - cluster ID with dot",
			opts: &forceUpgradeOptions{
				clusterID:      "test.cluster",
				nextRunMinutes: 10,
			},
			wantErr: true,
			errMsg:  "cluster ID 'test.cluster' contains invalid characters - only alphanumeric characters and hyphens are allowed",
		},
		// Basic clusters file test - just verify it can be used
		{
			name: "valid clusters file",
			opts: &forceUpgradeOptions{
				nextRunMinutes: 10,
				// clustersFile will be set in the test
			},
			needsClustersFile: true,
			wantErr:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.needsClustersFile {
				tmpClustersFile := filepath.Join(t.TempDir(), "clusters.json")
				fileContent := `{"clusters": ["cluster1", "cluster2", "cluster3"]}`
				err := os.WriteFile(tmpClustersFile, []byte(fileContent), 0600)
				if err != nil {
					t.Fatalf("Failed to create temp clusters file: %v", err)
				}
				tt.opts.clustersFile = tmpClustersFile
			}

			err := tt.opts.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if err.Error() != tt.errMsg {
					t.Errorf("validate() error message = %v, want %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestLoadServiceLogTemplate(t *testing.T) {
	// Create a temporary template file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test-template.json")
	testContent := `{"test": "template content"}`
	err := os.WriteFile(tmpFile, []byte(testContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create temp template file: %v", err)
	}

	tests := []struct {
		name                    string
		templateOrFile          string
		expectError             bool
		expectedUsingDefault    bool
		expectedContentContains string
	}{
		{
			name:                 "valid template name",
			templateOrFile:       "end-of-support",
			expectError:          false,
			expectedUsingDefault: true,
		},
		{
			name:                    "valid file path",
			templateOrFile:          tmpFile,
			expectError:             false,
			expectedUsingDefault:    false,
			expectedContentContains: "template content",
		},
		{
			name:           "invalid template name (treated as file path)",
			templateOrFile: "non-existent-template",
			expectError:    true,
		},
		{
			name:           "non-existent file path",
			templateOrFile: "/path/that/does/not/exist.json",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			templateBytes, usingDefaultTemplate, err := loadServiceLogTemplate(tt.templateOrFile)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				} else {
					if usingDefaultTemplate != tt.expectedUsingDefault {
						t.Errorf("Expected usingDefaultTemplate=%v, got %v", tt.expectedUsingDefault, usingDefaultTemplate)
					}
					if tt.expectedContentContains != "" && !strings.Contains(string(templateBytes), tt.expectedContentContains) {
						t.Errorf("Expected template content to contain '%s', got: %s", tt.expectedContentContains, string(templateBytes))
					}
				}
			}
		})
	}
}

func TestDetermineTargetVersion(t *testing.T) {
	opts := &forceUpgradeOptions{targetYStream: "4.15"}

	tests := []struct {
		name              string
		availableUpgrades []string
		expectedVersion   string
		expectError       bool
	}{
		{
			name:              "single matching version",
			availableUpgrades: []string{"4.15.1"},
			expectedVersion:   "4.15.1",
			expectError:       false,
		},
		{
			name:              "multiple matching versions - returns highest",
			availableUpgrades: []string{"4.15.1", "4.15.3", "4.15.2"},
			expectedVersion:   "4.15.3",
			expectError:       false,
		},
		{
			name:              "no matching versions",
			availableUpgrades: []string{"4.14.1", "4.16.1"},
			expectedVersion:   "",
			expectError:       false,
		},
		{
			name:              "mixed versions with matching Y-stream",
			availableUpgrades: []string{"4.14.5", "4.15.2", "4.16.1", "4.15.4"},
			expectedVersion:   "4.15.4",
			expectError:       false,
		},
		{
			name:              "invalid semver in upgrades",
			availableUpgrades: []string{"4.15.1", "invalid-version", "4.15.2"},
			expectedVersion:   "4.15.2",
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := opts.determineTargetVersion(tt.availableUpgrades)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if version != tt.expectedVersion {
				t.Errorf("Expected version %s, got %s", tt.expectedVersion, version)
			}
		})
	}
}
