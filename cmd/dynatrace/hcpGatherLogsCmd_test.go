package dynatrace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
			name:        "valid_directory_creation",
			destBaseDir: t.TempDir(),
			dirName:     "test-logs",
			expectError: false,
		},
		{
			name:        "invalid_base_directory",
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

			expectedDirPath := filepath.Join(tt.destBaseDir, fmt.Sprintf("hcp-logs-dump-%s", tt.dirName))

			if logsDir != expectedDirPath {
				t.Errorf("expected directory path %s but got %s for test: %s", expectedDirPath, logsDir, tt.name)
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
			name:        "valid_directory_and_files_creation",
			dirs:        []string{t.TempDir(), "test-add-dir"},
			filePaths:   []string{"file1.txt", "file2.log"},
			expectError: false,
		},
		{
			name:        "invalid_base_directory",
			dirs:        []string{"/invalid/path", "test-add-dir"},
			filePaths:   []string{"file1.txt", "file2.log"},
			expectError: true,
		},
		{
			name:        "no_files_to_create",
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

			expectedDirPath := filepath.Join(tt.dirs...)
			if dirPath != expectedDirPath {
				t.Errorf("expected dirPath %s, but got %s for test: %s", expectedDirPath, dirPath, tt.name)
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
			sortOrder:   "invalid",
			srcCluster:  "cluster1",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s-%s", tt.pod, tt.namespace), func(t *testing.T) {
			query, err := getPodQuery(tt.pod, tt.namespace, tt.since, tt.tail, tt.sortOrder, tt.srcCluster)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none for test: %s-%s", tt.pod, tt.namespace)
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("did not expect error but got: %v for test: %s-%s", err, tt.pod, tt.namespace)
				return
			}

			finalQuery := query.Build()

			if !strings.Contains(finalQuery, tt.pod) {
				t.Errorf("expected query to contain pod %s but it does not for test: %s-%s", tt.pod, tt.pod, tt.namespace)
			}

			if !strings.Contains(finalQuery, tt.namespace) {
				t.Errorf("expected query to contain namespace %s but it does not for test: %s-%s", tt.namespace, tt.pod, tt.namespace)
			}

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
			sortOrder:   "invalid",
			srcCluster:  "cluster1",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s-%s", tt.event, tt.namespace), func(t *testing.T) {
			query, err := getEventQuery(tt.event, tt.namespace, tt.since, tt.tail, tt.sortOrder, tt.srcCluster)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none for test: %s-%s", tt.event, tt.namespace)
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("did not expect error but got: %v for test: %s-%s", err, tt.event, tt.namespace)
				return
			}

			finalQuery := query.Build()

			if !strings.Contains(finalQuery, tt.event) {
				t.Errorf("expected query to contain event %s but it does not for test: %s-%s", tt.event, tt.event, tt.namespace)
			}

			if !strings.Contains(finalQuery, tt.namespace) {
				t.Errorf("expected query to contain namespace %s but it does not for test: %s-%s", tt.namespace, tt.event, tt.namespace)
			}

			if finalQuery == "" {
				t.Errorf("expected non-empty query but got an empty query for test: %s-%s", tt.event, tt.namespace)
			}
		})
	}
}
