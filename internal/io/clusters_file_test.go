package io

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAndValidateClustersFile(t *testing.T) {
	tests := []struct {
		name          string
		fileContent   string
		expectError   bool
		errorContains string
		expectedIDs   []string
	}{
		{
			name:        "valid cluster file with mixed ID types",
			fileContent: `{"clusters": ["2npb79qc3lqkrnn4g6u9cd9mqtlkb4gj", "testhcp", "a537f279-a25a-4b2b-b2d8-00b06d41ff2f"]}`,
			expectError: false,
			expectedIDs: []string{"2npb79qc3lqkrnn4g6u9cd9mqtlkb4gj", "testhcp", "a537f279-a25a-4b2b-b2d8-00b06d41ff2f"},
		},
		{
			name:        "valid cluster file with long alphanumeric cluster ID",
			fileContent: `{"clusters": ["2npb79qc3lqkrnn4g6u9cd9mqtlkb4gj"]}`,
			expectError: false,
			expectedIDs: []string{"2npb79qc3lqkrnn4g6u9cd9mqtlkb4gj"},
		},
		{
			name:        "valid cluster file with cluster name",
			fileContent: `{"clusters": ["testhcp"]}`,
			expectError: false,
			expectedIDs: []string{"testhcp"},
		},
		{
			name:        "valid cluster file with external ID (UUID)",
			fileContent: `{"clusters": ["a537f279-a25a-4b2b-b2d8-00b06d41ff2f"]}`,
			expectError: false,
			expectedIDs: []string{"a537f279-a25a-4b2b-b2d8-00b06d41ff2f"},
		},
		{
			name:        "valid cluster file with alphanumeric and hyphens",
			fileContent: `{"clusters": ["cluster-1", "test-cluster-123", "my-cluster-name"]}`,
			expectError: false,
			expectedIDs: []string{"cluster-1", "test-cluster-123", "my-cluster-name"},
		},
		{
			name:        "valid cluster file with single cluster",
			fileContent: `{"clusters": ["single-cluster"]}`,
			expectError: false,
			expectedIDs: []string{"single-cluster"},
		},
		{
			name:        "empty clusters array",
			fileContent: `{"clusters": []}`,
			expectError: false,
			expectedIDs: []string{},
		},
		{
			name:          "invalid cluster ID with special characters",
			fileContent:   `{"clusters": ["cluster1", "test@cluster!", "cluster3"]}`,
			expectError:   true,
			errorContains: "clusters file contains invalid cluster ID at index 1: 'test@cluster!' - only alphanumeric characters and hyphens are allowed",
		},
		{
			name:          "invalid cluster ID with underscore",
			fileContent:   `{"clusters": ["cluster1", "test_cluster", "cluster3"]}`,
			expectError:   true,
			errorContains: "clusters file contains invalid cluster ID at index 1: 'test_cluster' - only alphanumeric characters and hyphens are allowed",
		},
		{
			name:          "invalid cluster ID with dot",
			fileContent:   `{"clusters": ["cluster1", "test.cluster", "cluster3"]}`,
			expectError:   true,
			errorContains: "clusters file contains invalid cluster ID at index 1: 'test.cluster' - only alphanumeric characters and hyphens are allowed",
		},
		{
			name:          "invalid cluster ID with spaces",
			fileContent:   `{"clusters": ["cluster1", "test cluster", "cluster3"]}`,
			expectError:   true,
			errorContains: "clusters file contains invalid cluster ID at index 1: 'test cluster' - only alphanumeric characters and hyphens are allowed",
		},
		{
			name:          "empty cluster ID",
			fileContent:   `{"clusters": ["cluster1", "", "cluster3"]}`,
			expectError:   true,
			errorContains: "clusters file contains invalid cluster ID at index 1: '' - only alphanumeric characters and hyphens are allowed",
		},
		{
			name:          "whitespace-only cluster ID",
			fileContent:   `{"clusters": ["cluster1", "   ", "cluster3"]}`,
			expectError:   true,
			errorContains: "clusters file contains invalid cluster ID at index 1: '   ' - only alphanumeric characters and hyphens are allowed",
		},
		{
			name:          "cluster ID with forward slash",
			fileContent:   `{"clusters": ["cluster/path"]}`,
			expectError:   true,
			errorContains: "clusters file contains invalid cluster ID at index 0: 'cluster/path' - only alphanumeric characters and hyphens are allowed",
		},
		{
			name:          "cluster ID with colon",
			fileContent:   `{"clusters": ["cluster:name"]}`,
			expectError:   true,
			errorContains: "clusters file contains invalid cluster ID at index 0: 'cluster:name' - only alphanumeric characters and hyphens are allowed",
		},
		{
			name:          "invalid JSON - missing quotes",
			fileContent:   `{clusters: [cluster1, cluster2]}`,
			expectError:   true,
			errorContains: "failed to parse clusters file",
		},
		{
			name:          "invalid JSON - malformed",
			fileContent:   `{"clusters": [cluster1]}`,
			expectError:   true,
			errorContains: "failed to parse clusters file",
		},
		{
			name:          "invalid JSON - missing closing brace",
			fileContent:   `{"clusters": ["cluster1", "cluster2"`,
			expectError:   true,
			errorContains: "failed to parse clusters file",
		},
		{
			name:          "wrong JSON structure - clusters not an array",
			fileContent:   `{"clusters": "cluster1"}`,
			expectError:   true,
			errorContains: "failed to parse clusters file",
		},
		{
			name:        "wrong JSON structure - missing clusters field",
			fileContent: `{"items": ["cluster1", "cluster2"]}`,
			expectError: false,
			expectedIDs: []string{},
		},
		{
			name:          "empty file",
			fileContent:   ``,
			expectError:   true,
			errorContains: "failed to parse clusters file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "clusters.json")

			err := os.WriteFile(tmpFile, []byte(tt.fileContent), 0600)
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			clusters, err := ParseAndValidateClustersFile(tmpFile)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				} else {
					// Verify the returned cluster IDs match expected
					if len(clusters) != len(tt.expectedIDs) {
						t.Errorf("Expected %d clusters, got %d", len(tt.expectedIDs), len(clusters))
					} else {
						for i, expectedID := range tt.expectedIDs {
							if clusters[i] != expectedID {
								t.Errorf("Expected cluster ID at index %d to be '%s', got '%s'", i, expectedID, clusters[i])
							}
						}
					}
				}
			}
		})
	}
}

func TestParseAndValidateClustersFileNotFound(t *testing.T) {
	nonExistentFile := "/path/that/does/not/exist/clusters.json"

	_, err := ParseAndValidateClustersFile(nonExistentFile)

	if err == nil {
		t.Error("Expected error for non-existent file, but got none")
	}

	if !strings.Contains(err.Error(), "failed to read clusters file") {
		t.Errorf("Expected error to contain 'failed to read clusters file', got: %v", err)
	}
}

func TestValidClusterIDRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "long alphanumeric cluster ID",
			input:    "2npb79qc3lqkrnn4g6u9cd9mqtlkb4gj",
			expected: true,
		},
		{
			name:     "cluster name",
			input:    "testhcp",
			expected: true,
		},
		{
			name:     "external ID (UUID)",
			input:    "a537f279-a25a-4b2b-b2d8-00b06d41ff2f",
			expected: true,
		},
		{
			name:     "simple alphanumeric",
			input:    "cluster123",
			expected: true,
		},
		{
			name:     "alphanumeric with hyphens",
			input:    "test-cluster-123",
			expected: true,
		},
		{
			name:     "single character",
			input:    "a",
			expected: true,
		},
		{
			name:     "single number",
			input:    "1",
			expected: true,
		},
		{
			name:     "single hyphen",
			input:    "-",
			expected: true,
		},
		{
			name:     "starts with hyphen",
			input:    "-cluster",
			expected: true,
		},
		{
			name:     "ends with hyphen",
			input:    "cluster-",
			expected: true,
		},
		{
			name:     "multiple consecutive hyphens",
			input:    "test--cluster",
			expected: true,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "contains underscore",
			input:    "test_cluster",
			expected: false,
		},
		{
			name:     "contains dot",
			input:    "test.cluster",
			expected: false,
		},
		{
			name:     "contains space",
			input:    "test cluster",
			expected: false,
		},
		{
			name:     "contains special characters",
			input:    "test@cluster!",
			expected: false,
		},
		{
			name:     "contains forward slash",
			input:    "cluster/path",
			expected: false,
		},
		{
			name:     "contains colon",
			input:    "cluster:name",
			expected: false,
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: false,
		},
		{
			name:     "tabs",
			input:    "\t",
			expected: false,
		},
		{
			name:     "newlines",
			input:    "\n",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validClusterIDRegex.MatchString(tt.input)
			if result != tt.expected {
				t.Errorf("For input '%s', expected %v, got %v", tt.input, tt.expected, result)
			}
		})
	}
}
