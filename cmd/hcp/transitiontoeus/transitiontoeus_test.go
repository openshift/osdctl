package transitiontoeus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTransitionOptionsValidation(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test-template.json")
	err := os.WriteFile(tmpFile, []byte(`{"test": "template"}`), 0600)
	if err != nil {
		t.Fatalf("Failed to create temp file for test: %v", err)
	}

	tmpClustersFile := filepath.Join(tmpDir, "clusters.json")
	err = os.WriteFile(tmpClustersFile, []byte(`{"clusters":["cluster1","cluster2"]}`), 0600)
	if err != nil {
		t.Fatalf("Failed to create temp clusters file for test: %v", err)
	}

	tests := []struct {
		name    string
		opts    *transitionOptions
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid with cluster ID",
			opts: &transitionOptions{
				clusterID: "test-cluster",
			},
			wantErr: false,
		},
		{
			name: "valid with clusters file",
			opts: &transitionOptions{
				clustersFile: tmpClustersFile,
			},
			wantErr: false,
		},
		{
			name:    "invalid - no cluster targeting",
			opts:    &transitionOptions{},
			wantErr: true,
			errMsg:  "no cluster identifier has been found, please specify either --cluster-id or --clusters-file",
		},
		{
			name: "invalid - both cluster ID and file",
			opts: &transitionOptions{
				clusterID:    "test-cluster",
				clustersFile: tmpClustersFile,
			},
			wantErr: true,
			errMsg:  "cannot specify both --cluster-id and --clusters-file, choose one",
		},
		{
			name: "valid - cluster ID with alphanumeric and hyphens",
			opts: &transitionOptions{
				clusterID: "abc123-test-cluster-456",
			},
			wantErr: false,
		},
		{
			name: "invalid - cluster ID with special characters",
			opts: &transitionOptions{
				clusterID: "cluster@123",
			},
			wantErr: true,
			errMsg:  "cluster ID 'cluster@123' contains invalid characters",
		},
		{
			name: "invalid - cluster ID with spaces",
			opts: &transitionOptions{
				clusterID: "cluster 123",
			},
			wantErr: true,
			errMsg:  "cluster ID 'cluster 123' contains invalid characters",
		},
		{
			name: "valid - dry-run enabled",
			opts: &transitionOptions{
				clusterID: "test-cluster",
				dryRun:    true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("validate() error message = %v, want to contain %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestClusterIDValidation(t *testing.T) {
	tests := []struct {
		name      string
		clusterID string
		wantValid bool
	}{
		{
			name:      "valid - alphanumeric",
			clusterID: "abc123",
			wantValid: true,
		},
		{
			name:      "valid - with hyphens",
			clusterID: "abc-123-def",
			wantValid: true,
		},
		{
			name:      "valid - long cluster ID",
			clusterID: "2abcdefg3hijklmn4opqrstu5vwxyz67",
			wantValid: true,
		},
		{
			name:      "invalid - with underscore",
			clusterID: "cluster_123",
			wantValid: false,
		},
		{
			name:      "invalid - with special characters",
			clusterID: "cluster@123",
			wantValid: false,
		},
		{
			name:      "invalid - with spaces",
			clusterID: "cluster 123",
			wantValid: false,
		},
		{
			name:      "invalid - with dots",
			clusterID: "cluster.123",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := validClusterIDRegex.MatchString(tt.clusterID)
			if matched != tt.wantValid {
				t.Errorf("validClusterIDRegex.MatchString(%q) = %v, want %v", tt.clusterID, matched, tt.wantValid)
			}
		})
	}
}

func TestServiceLogTemplates(t *testing.T) {
	// Test that all expected templates are defined
	expectedTemplates := []string{"success", "attempted"}

	for _, template := range expectedTemplates {
		t.Run("template_exists_"+template, func(t *testing.T) {
			if _, exists := serviceLogTemplates[template]; !exists {
				t.Errorf("Expected service log template %q to be defined", template)
			}
		})
	}

	// Test that all template URLs follow the expected pattern
	for name, url := range serviceLogTemplates {
		t.Run("template_url_format_"+name, func(t *testing.T) {
			if !contains(url, "github.com") && !contains(url, "githubusercontent.com") {
				t.Errorf("Template URL for %q does not appear to be a GitHub URL: %s", name, url)
			}
			if !contains(url, "/hcp/") {
				t.Errorf("Template URL for %q does not contain '/hcp/' path: %s", name, url)
			}
			if !contains(url, ".json") {
				t.Errorf("Template URL for %q does not end with .json: %s", name, url)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && s[:len(substr)] == substr) ||
		containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
