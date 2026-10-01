package transitiontoeus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
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

func TestPrintPoliciesForManualRestore(t *testing.T) {
	// Create a temporary directory for test output files
	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWd); err != nil {
			t.Errorf("Failed to restore working directory: %v", err)
		}
	}()

	// Change to temp directory so the output file is created there
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Create test policies using OCM SDK types
	policy1, err := v1.NewControlPlaneUpgradePolicy().
		Schedule("0 2 * * 1").
		ScheduleType(v1.ScheduleTypeAutomatic).
		UpgradeType("ControlPlane").
		EnableMinorVersionUpgrades(true).
		Build()
	if err != nil {
		t.Fatalf("Failed to build test policy 1: %v", err)
	}

	policy2, err := v1.NewControlPlaneUpgradePolicy().
		Schedule("0 3 * * 2").
		ScheduleType(v1.ScheduleTypeManual).
		UpgradeType("ControlPlane").
		EnableMinorVersionUpgrades(false).
		Version("4.14.5").
		Build()
	if err != nil {
		t.Fatalf("Failed to build test policy 2: %v", err)
	}

	policies := []*v1.ControlPlaneUpgradePolicy{policy1, policy2}
	clusterID := "test-cluster-123"

	// Call the function
	printPoliciesForManualRestore(policies, clusterID)

	// Find the generated file
	files, err := filepath.Glob("unrestored_policies_*.sh")
	if err != nil {
		t.Fatalf("Failed to glob for output files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Expected 1 output file, got %d", len(files))
	}

	// Read and validate the file
	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	contentStr := string(content)

	// Validate file format
	if !strings.HasPrefix(contentStr, "#!/bin/bash\n") {
		t.Error("File should start with bash shebang")
	}
	if !contains(contentStr, clusterID) {
		t.Error("File should contain cluster ID")
	}
	if !contains(contentStr, "0 2 * * 1") {
		t.Error("File should contain policy 1 schedule")
	}
	if !contains(contentStr, "0 3 * * 2") {
		t.Error("File should contain policy 2 schedule")
	}
	if !contains(contentStr, "automatic") {
		t.Error("File should contain automatic schedule type")
	}
	if !contains(contentStr, "manual") {
		t.Error("File should contain manual schedule type")
	}
	if !contains(contentStr, "4.14.5") {
		t.Error("File should contain version for manual policy")
	}
	if !contains(contentStr, "ocm post") {
		t.Error("File should contain ocm post commands")
	}

	// Check file is executable
	fileInfo, err := os.Stat(files[0])
	if err != nil {
		t.Fatalf("Failed to stat output file: %v", err)
	}
	if fileInfo.Mode()&0111 == 0 {
		t.Error("File should be executable")
	}
}

func TestResolveTemplate(t *testing.T) {
	if got := resolveTemplate("success"); !strings.Contains(got, "eus_transition_success.json") {
		t.Errorf("resolveTemplate(\"success\") = %q, want URL containing eus_transition_success.json", got)
	}
	if got := resolveTemplate("/custom/path.json"); got != "/custom/path.json" {
		t.Errorf("resolveTemplate(\"/custom/path.json\") = %q, want \"/custom/path.json\"", got)
	}
}

func TestClusterProcessResult(t *testing.T) {
	// Test the result struct tracking
	tests := []struct {
		name               string
		policyWasModified  bool
		policyWasRestored  bool
		unrestoredPolicies int
		expectCritical     bool
	}{
		{
			name:               "no modification",
			policyWasModified:  false,
			policyWasRestored:  false,
			unrestoredPolicies: 0,
			expectCritical:     false,
		},
		{
			name:               "modified and restored",
			policyWasModified:  true,
			policyWasRestored:  true,
			unrestoredPolicies: 0,
			expectCritical:     false,
		},
		{
			name:               "CRITICAL - modified but not restored",
			policyWasModified:  true,
			policyWasRestored:  false,
			unrestoredPolicies: 2,
			expectCritical:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &clusterProcessResult{
				policyWasModified: tt.policyWasModified,
				policyWasRestored: tt.policyWasRestored,
			}

			if tt.unrestoredPolicies > 0 {
				// Create mock policies
				for i := 0; i < tt.unrestoredPolicies; i++ {
					policy, _ := v1.NewControlPlaneUpgradePolicy().
						Schedule("0 2 * * 1").
						ScheduleType(v1.ScheduleTypeAutomatic).
						UpgradeType("ControlPlane").
						EnableMinorVersionUpgrades(true).
						Build()
					result.unrestoredPolicies = append(result.unrestoredPolicies, policy)
				}
			}

			// Verify critical state
			isCritical := result.policyWasModified && !result.policyWasRestored && len(result.unrestoredPolicies) > 0
			if isCritical != tt.expectCritical {
				t.Errorf("Critical state = %v, want %v", isCritical, tt.expectCritical)
			}
		})
	}
}

func TestPolicyDetails(t *testing.T) {
	// Test the policyDetails struct
	policy1, err := v1.NewControlPlaneUpgradePolicy().
		Schedule("0 2 * * 1").
		ScheduleType(v1.ScheduleTypeAutomatic).
		UpgradeType("ControlPlane").
		EnableMinorVersionUpgrades(true).
		Build()
	if err != nil {
		t.Fatalf("Failed to build test policy: %v", err)
	}

	tests := []struct {
		name                 string
		hasRecurringPolicies bool
		policyCount          int
	}{
		{
			name:                 "no recurring policies",
			hasRecurringPolicies: false,
			policyCount:          0,
		},
		{
			name:                 "has recurring policies",
			hasRecurringPolicies: true,
			policyCount:          1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := &policyDetails{
				hasRecurringPolicies: tt.hasRecurringPolicies,
			}

			if tt.policyCount > 0 {
				details.recurringPolicies = []*v1.ControlPlaneUpgradePolicy{policy1}
			}

			if details.hasRecurringPolicies != tt.hasRecurringPolicies {
				t.Errorf("hasRecurringPolicies = %v, want %v", details.hasRecurringPolicies, tt.hasRecurringPolicies)
			}
			if len(details.recurringPolicies) != tt.policyCount {
				t.Errorf("policy count = %v, want %v", len(details.recurringPolicies), tt.policyCount)
			}
		})
	}
}
