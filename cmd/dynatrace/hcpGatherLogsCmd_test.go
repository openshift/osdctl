package dynatrace

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupGatherDir(t *testing.T) {
	tests := []struct {
		name        string
		destBaseDir string
		dirName     string
		expectError bool
	}{
		{
			name:        "Valid directory creation",
			destBaseDir: t.TempDir(),
			dirName:     "test-logs",
			expectError: false,
		},
		{
			name:        "Invalid base directory",
			destBaseDir: "/invalid/path",
			dirName:     "test-logs",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logsDir, err := setupGatherDir(tt.destBaseDir, tt.dirName)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none for test: %s", tt.name)
				}
				return
			}

			if err != nil {
				t.Errorf("did not expect an error but got: %v for test: %s", err, tt.name)
				return
			}

			if _, err := os.Stat(logsDir); os.IsNotExist(err) {
				t.Errorf("expected directory %s to be created but it does not exist for test: %s", logsDir, tt.name)
			}
		})
	}
}
func TestAddDir(t *testing.T) {
	tests := []struct {
		name        string
		dirs        []string
		filePaths   []string
		expectError bool
	}{
		{
			name:        "Valid directory and files creation",
			dirs:        []string{t.TempDir(), "test-add-dir"},
			filePaths:   []string{"file1.txt", "file2.log"},
			expectError: false,
		},
		{
			name:        "Invalid base directory",
			dirs:        []string{"/invalid/path", "test-add-dir"},
			filePaths:   []string{"file1.txt", "file2.log"},
			expectError: true,
		},
		{
			name:        "No files to create",
			dirs:        []string{t.TempDir(), "test-empty-files"},
			filePaths:   []string{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirPath, err := addDir(tt.dirs, tt.filePaths)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none for test: %s", tt.name)
				}
				return
			}

			if err != nil {
				t.Errorf("did not expect an error but got: %v for test: %s", err, tt.name)
				return
			}

			if _, err := os.Stat(dirPath); os.IsNotExist(err) {
				t.Errorf("expected directory %s to be created but it does not exist for test: %s", dirPath, tt.name)
			}

			for _, file := range tt.filePaths {
				filePath := filepath.Join(dirPath, file)
				if _, err := os.Stat(filePath); os.IsNotExist(err) {
					t.Errorf("expected file %s to be created but it does not exist for test: %s", filePath, tt.name)
				}
			}
		})
	}
}
func TestGetPodQuery(t *testing.T) {
	tests := []struct {
		pod         string
		namespace   string
		since       int
		tail        int
		sortOrder   string
		srcCluster  string
		expectError bool
	}{
		{
			pod:         "test-pod",
			namespace:   "test-namespace",
			since:       24,
			tail:        100,
			sortOrder:   "asc",
			srcCluster:  "cluster1",
			expectError: false,
		},
		{
			pod:         "",
			namespace:   "test-namespace",
			since:       24,
			tail:        100,
			sortOrder:   "asc",
			srcCluster:  "cluster1",
			expectError: false,
		},
		{
			pod:         "test-pod",
			namespace:   "",
			since:       24,
			tail:        100,
			sortOrder:   "asc",
			srcCluster:  "cluster1",
			expectError: false,
		},
		{
			pod:         "test-pod",
			namespace:   "test-namespace",
			since:       24,
			tail:        100,
			sortOrder:   "invalid", // Invalid sort order
			srcCluster:  "cluster1",
			expectError: true, // Expected error for invalid sort order
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s-%s", tt.pod, tt.namespace), func(t *testing.T) {
			// Call the function
			query, err := getPodQuery(tt.pod, tt.namespace, tt.since, tt.tail, tt.sortOrder, tt.srcCluster)

			// Check for expected error
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none for test: %s-%s", tt.pod, tt.namespace)
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("did not expect error but got: %v for test: %s-%s", err, tt.pod, tt.namespace)
				return
			}

			// Build the final query
			finalQuery := query.Build()

			// Ensure query is non-empty
			if finalQuery == "" {
				t.Errorf("expected non-empty query but got an empty query for test: %s-%s", tt.pod, tt.namespace)
			}
		})
	}
}
func TestGetEventQuery(t *testing.T) {
	tests := []struct {
		event       string
		namespace   string
		since       int
		tail        int
		sortOrder   string
		srcCluster  string
		expectError bool
	}{
		{
			event:       "test-event",
			namespace:   "test-namespace",
			since:       24,
			tail:        100,
			sortOrder:   "asc",
			srcCluster:  "cluster1",
			expectError: false,
		},
		{
			event:       "",
			namespace:   "test-namespace",
			since:       24,
			tail:        100,
			sortOrder:   "asc",
			srcCluster:  "cluster1",
			expectError: false,
		},
		{
			event:       "test-event",
			namespace:   "",
			since:       24,
			tail:        100,
			sortOrder:   "asc",
			srcCluster:  "cluster1",
			expectError: false,
		},
		{
			event:       "test-event",
			namespace:   "test-namespace",
			since:       24,
			tail:        100,
			sortOrder:   "invalid", // Invalid sort order
			srcCluster:  "cluster1",
			expectError: true, // Expected error for invalid sort order
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s-%s", tt.event, tt.namespace), func(t *testing.T) {
			// Call the function
			query, err := getEventQuery(tt.event, tt.namespace, tt.since, tt.tail, tt.sortOrder, tt.srcCluster)

			// Check for expected error
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none for test: %s-%s", tt.event, tt.namespace)
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("did not expect error but got: %v for test: %s-%s", err, tt.event, tt.namespace)
				return
			}

			// Build the final query
			finalQuery := query.Build()

			// Ensure query is non-empty
			if finalQuery == "" {
				t.Errorf("expected non-empty query but got an empty query for test: %s-%s", tt.event, tt.namespace)
			}
		})
	}
}
