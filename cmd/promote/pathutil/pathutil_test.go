package pathutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveAppYmlPath(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) (gitDir, saasFile, component string)
		expectedPath  string
		expectError   bool
		errorContains string
	}{
		{
			name: "flat_structure_file_exists",
			setup: func(t *testing.T) (string, string, string) {
				tmpDir := t.TempDir()
				appYmlPath := filepath.Join(tmpDir, "data", "services", "my-service", "app.yml")
				require.NoError(t, os.MkdirAll(filepath.Dir(appYmlPath), 0755))
				require.NoError(t, os.WriteFile(appYmlPath, []byte("test"), 0600))

				saasFile := filepath.Join(tmpDir, "data", "services", "my-service", "cicd", "saas", "saas-my-service.yaml")
				return tmpDir, saasFile, "my-service"
			},
			expectedPath: "data/services/my-service/app.yml",
			expectError:  false,
		},
		{
			name: "nested_osd_operators_file_exists",
			setup: func(t *testing.T) (string, string, string) {
				tmpDir := t.TempDir()
				appYmlPath := filepath.Join(tmpDir, "data", "services", "osd-operators", "managed-cluster-config", "app.yml")
				require.NoError(t, os.MkdirAll(filepath.Dir(appYmlPath), 0755))
				require.NoError(t, os.WriteFile(appYmlPath, []byte("test"), 0600))

				saasFile := filepath.Join(tmpDir, "data", "services", "osd-operators", "cicd", "saas", "saas-managed-cluster-config.yaml")
				return tmpDir, saasFile, "managed-cluster-config"
			},
			expectedPath: "data/services/osd-operators/managed-cluster-config/app.yml",
			expectError:  false,
		},
		{
			name: "nested_backplane_file_exists",
			setup: func(t *testing.T) (string, string, string) {
				tmpDir := t.TempDir()
				appYmlPath := filepath.Join(tmpDir, "data", "services", "backplane", "backplane-api", "app.yml")
				require.NoError(t, os.MkdirAll(filepath.Dir(appYmlPath), 0755))
				require.NoError(t, os.WriteFile(appYmlPath, []byte("test"), 0600))

				saasFile := filepath.Join(tmpDir, "data", "services", "backplane", "cicd", "saas", "saas-backplane-api.yaml")
				return tmpDir, saasFile, "backplane-api"
			},
			expectedPath: "data/services/backplane/backplane-api/app.yml",
			expectError:  false,
		},
		{
			name: "derivation_fails_but_filesystem_search_succeeds",
			setup: func(t *testing.T) (string, string, string) {
				tmpDir := t.TempDir()
				appYmlPath := filepath.Join(tmpDir, "data", "services", "osd-operators", "test-operator", "app.yml")
				require.NoError(t, os.MkdirAll(filepath.Dir(appYmlPath), 0755))
				require.NoError(t, os.WriteFile(appYmlPath, []byte("test"), 0600))

				saasFile := filepath.Join(tmpDir, "unusual", "path", "saas-test-operator.yaml")
				return tmpDir, saasFile, "test-operator"
			},
			expectedPath: "data/services/osd-operators/test-operator/app.yml",
			expectError:  false,
		},
		{
			name: "file_does_not_exist_returns_derived_path_anyway",
			setup: func(t *testing.T) (string, string, string) {
				tmpDir := t.TempDir()
				saasFile := filepath.Join(tmpDir, "data", "services", "new-service", "cicd", "saas", "saas-new-service.yaml")
				return tmpDir, saasFile, "new-service"
			},
			expectedPath: "data/services/new-service/app.yml",
			expectError:  false,
		},
		{
			name: "completely_invalid_path_returns_error",
			setup: func(t *testing.T) (string, string, string) {
				tmpDir := t.TempDir()
				saasFile := filepath.Join(tmpDir, "invalid", "path", "saas-test.yaml")
				return tmpDir, saasFile, "test"
			},
			expectError:   true,
			errorContains: "could not determine app.yml path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitDir, saasFile, component := tt.setup(t)

			result, err := DeriveAppYmlPath(gitDir, saasFile, component)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				relPath, err := filepath.Rel(gitDir, result)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedPath, relPath)
			}
		})
	}
}

func TestDeriveFromSaasPath(t *testing.T) {
	tests := []struct {
		name          string
		gitDirectory  string
		saasFile      string
		componentName string
		expectedPath  string
		expectError   bool
		errorContains string
	}{
		{
			name:          "flat_structure",
			gitDirectory:  "/app-interface",
			saasFile:      "/app-interface/data/services/my-service/cicd/saas/saas-my-service.yaml",
			componentName: "my-service",
			expectedPath:  "/app-interface/data/services/my-service/app.yml",
			expectError:   false,
		},
		{
			name:          "nested_osd_operators",
			gitDirectory:  "/app-interface",
			saasFile:      "/app-interface/data/services/osd-operators/cicd/saas/saas-managed-cluster-config.yaml",
			componentName: "managed-cluster-config",
			expectedPath:  "/app-interface/data/services/osd-operators/managed-cluster-config/app.yml",
			expectError:   false,
		},
		{
			name:          "nested_backplane",
			gitDirectory:  "/app-interface",
			saasFile:      "/app-interface/data/services/backplane/cicd/saas/saas-backplane-api.yaml",
			componentName: "backplane-api",
			expectedPath:  "/app-interface/data/services/backplane/backplane-api/app.yml",
			expectError:   false,
		},
		{
			name:          "path_without_services_fails",
			gitDirectory:  "/app-interface",
			saasFile:      "/app-interface/data/other/cicd/saas/saas-test.yaml",
			componentName: "test",
			expectError:   true,
			errorContains: "services' directory not found",
		},
		{
			name:          "cicd_parent_uses_flat_structure",
			gitDirectory:  "/app-interface",
			saasFile:      "/app-interface/data/services/cicd/saas/saas-test.yaml",
			componentName: "test",
			expectedPath:  "/app-interface/data/services/test/app.yml",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := deriveFromSaasPath(tt.gitDirectory, tt.saasFile, tt.componentName)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPath, result)
			}
		})
	}
}

func TestSearchFilesystem(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) (gitDir, component string)
		expectedPath  string
		expectError   bool
		errorContains string
	}{
		{
			name: "finds_flat_structure",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				appYmlPath := filepath.Join(tmpDir, "data", "services", "my-service", "app.yml")
				require.NoError(t, os.MkdirAll(filepath.Dir(appYmlPath), 0755))
				require.NoError(t, os.WriteFile(appYmlPath, []byte("test"), 0600))
				return tmpDir, "my-service"
			},
			expectedPath: "data/services/my-service/app.yml",
			expectError:  false,
		},
		{
			name: "finds_osd_operators",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				appYmlPath := filepath.Join(tmpDir, "data", "services", "osd-operators", "test-operator", "app.yml")
				require.NoError(t, os.MkdirAll(filepath.Dir(appYmlPath), 0755))
				require.NoError(t, os.WriteFile(appYmlPath, []byte("test"), 0600))
				return tmpDir, "test-operator"
			},
			expectedPath: "data/services/osd-operators/test-operator/app.yml",
			expectError:  false,
		},
		{
			name: "finds_backplane",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				appYmlPath := filepath.Join(tmpDir, "data", "services", "backplane", "backplane-api", "app.yml")
				require.NoError(t, os.MkdirAll(filepath.Dir(appYmlPath), 0755))
				require.NoError(t, os.WriteFile(appYmlPath, []byte("test"), 0600))
				return tmpDir, "backplane-api"
			},
			expectedPath: "data/services/backplane/backplane-api/app.yml",
			expectError:  false,
		},
		{
			name: "file_not_found_returns_error",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				return tmpDir, "nonexistent"
			},
			expectError:   true,
			errorContains: "app.yml not found",
		},
		{
			name: "prefers_first_match",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				flatPath := filepath.Join(tmpDir, "data", "services", "duplicate", "app.yml")
				require.NoError(t, os.MkdirAll(filepath.Dir(flatPath), 0755))
				require.NoError(t, os.WriteFile(flatPath, []byte("flat"), 0600))

				nestedPath := filepath.Join(tmpDir, "data", "services", "osd-operators", "duplicate", "app.yml")
				require.NoError(t, os.MkdirAll(filepath.Dir(nestedPath), 0755))
				require.NoError(t, os.WriteFile(nestedPath, []byte("nested"), 0600))

				return tmpDir, "duplicate"
			},
			expectedPath: "data/services/duplicate/app.yml",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitDir, component := tt.setup(t)

			result, err := searchFilesystem(gitDir, component)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				relPath, err := filepath.Rel(gitDir, result)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedPath, relPath)
			}
		})
	}
}
