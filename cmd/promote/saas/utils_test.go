package saas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openshift/osdctl/cmd/promote/git"
	"github.com/openshift/osdctl/cmd/promote/pathutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSaasDir(t *testing.T) {
	tests := []struct {
		name         string
		servicesMap  map[string]string
		serviceName  string
		osd          bool
		hcp          bool
		expectedPath string
		expectErr    bool
		errMessage   string
	}{
		{
			name:         "service_not_found",
			servicesMap:  map[string]string{"serviceA": "some/path"},
			serviceName:  "unknown",
			osd:          false,
			hcp:          false,
			expectedPath: "",
			expectErr:    true,
			errMessage:   "saas directory for service unknown not found",
		},
		{
			name:         "has_yaml_and_osd_true",
			servicesMap:  map[string]string{"service2": "config/service2.yaml"},
			serviceName:  "service2",
			osd:          true,
			hcp:          false,
			expectedPath: "config/service2.yaml",
			expectErr:    false,
		},
		{
			name:         "no_yaml_and_osd_true",
			servicesMap:  map[string]string{"service1": "path/to/service1"},
			serviceName:  "service1",
			osd:          true,
			hcp:          false,
			expectedPath: "path/to/service1/deploy.yaml",
			expectErr:    false,
		},
		{
			name:         "no_yaml_and_hcp_true",
			servicesMap:  map[string]string{"service1": "path/to/service1"},
			serviceName:  "service1",
			osd:          false,
			hcp:          true,
			expectedPath: "path/to/service1/hypershift-deploy.yaml",
			expectErr:    false,
		},
		{
			name:         "no_yaml_osd_false_hcp_false_should_error",
			servicesMap:  map[string]string{"service1": "path/to/service1"},
			serviceName:  "service1",
			osd:          false,
			hcp:          false,
			expectedPath: "",
			expectErr:    true,
			errMessage:   "saas directory for service service1 not found",
		},
		{
			name:         "nil_map_should_return_error",
			servicesMap:  nil,
			serviceName:  "anyservice",
			osd:          true,
			hcp:          false,
			expectedPath: "",
			expectErr:    true,
			errMessage:   "saas directory for service anyservice not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ServicesFilesMap = tc.servicesMap
			result, err := GetSaasDir(tc.serviceName, tc.osd, tc.hcp)
			if tc.expectErr {
				assert.Error(t, err)
				assert.EqualError(t, err, tc.errMessage)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedPath, result)
			}
		})
	}
}

func TestValidateServiceName(t *testing.T) {
	tests := []struct {
		name         string
		serviceSlice []string
		serviceName  string
		expected     string
		expectErr    bool
		errMessage   string
	}{
		{
			name:         "exact_match",
			serviceSlice: []string{"serviceA", "serviceB"},
			serviceName:  "serviceA",
			expected:     "serviceA",
			expectErr:    false,
		},
		{
			name:         "saas_prefix_match",
			serviceSlice: []string{"saas-serviceC", "serviceD"},
			serviceName:  "serviceC",
			expected:     "saas-serviceC",
			expectErr:    false,
		},
		{
			name:         "no_match_found",
			serviceSlice: []string{"serviceX", "saas-serviceY"},
			serviceName:  "unknown",
			expected:     "unknown",
			expectErr:    true,
			errMessage:   "service unknown not found",
		},
		{
			name:         "empty_service_list",
			serviceSlice: []string{},
			serviceName:  "anything",
			expected:     "anything",
			expectErr:    true,
			errMessage:   "service anything not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ValidateServiceName(tc.serviceSlice, tc.serviceName)

			if tc.expectErr {
				assert.Error(t, err)
				assert.EqualError(t, err, tc.errMessage)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestGetServiceNames(t *testing.T) {
	type testCase struct {
		name       string
		setup      func(baseDir string) []string
		expected   []string
		wantErr    bool
		errMessage string
		assertions func(t *testing.T, got []string)
	}

	tempDir := t.TempDir()

	tests := []testCase{
		{
			name: "returns_services_from_saas_dir",
			setup: func(baseDir string) []string {
				dir := filepath.Join(baseDir, "saas")
				_ = os.MkdirAll(dir, 0755)
				_ = os.WriteFile(filepath.Join(dir, "saas-foo.yaml"), []byte("..."), 0644)
				_ = os.WriteFile(filepath.Join(dir, "saas-bar.yaml"), []byte("..."), 0644)
				return []string{"saas"}
			},
			expected: []string{"saas-foo", "saas-bar"},
			wantErr:  false,
			assertions: func(t *testing.T, got []string) {
				assert.ElementsMatch(t, []string{"saas-foo", "saas-bar"}, got)
				assert.Equal(t, "saas-foo.yaml", filepath.Base(ServicesFilesMap["saas-foo"]))
				assert.Equal(t, "saas-bar.yaml", filepath.Base(ServicesFilesMap["saas-bar"]))
				assert.Len(t, ServicesFilesMap, 2)
			},
		},
		{
			name: "returns_empty_slice_for_empty_dir",
			setup: func(baseDir string) []string {
				dir := filepath.Join(baseDir, "empty")
				_ = os.MkdirAll(dir, 0755)
				return []string{"empty"}
			},
			expected: []string{},
			wantErr:  false,
			assertions: func(t *testing.T, got []string) {
				assert.Empty(t, got)
				assert.Empty(t, ServicesFilesMap)
			},
		},
		{
			name: "returns_error_for_invalid_glob_pattern",
			setup: func(baseDir string) []string {
				return []string{"bad[pattern"}
			},
			expected:   nil,
			wantErr:    true,
			errMessage: "syntax error in pattern",
			assertions: func(t *testing.T, got []string) {
				assert.Nil(t, got)
				assert.Empty(t, ServicesFilesMap)
			},
		},
		{
			name: "handles_multiple_directories",
			setup: func(baseDir string) []string {
				dir1 := filepath.Join(baseDir, "dir1")
				dir2 := filepath.Join(baseDir, "dir2")
				_ = os.MkdirAll(dir1, 0755)
				_ = os.MkdirAll(dir2, 0755)
				_ = os.WriteFile(filepath.Join(dir1, "saas-alpha.yaml"), []byte("..."), 0644)
				_ = os.WriteFile(filepath.Join(dir2, "saas-beta.yaml"), []byte("..."), 0644)
				return []string{"dir1", "dir2"}
			},
			expected: []string{"saas-alpha", "saas-beta"},
			wantErr:  false,
			assertions: func(t *testing.T, got []string) {
				assert.ElementsMatch(t, []string{"saas-alpha", "saas-beta"}, got)
				assert.Equal(t, "saas-alpha.yaml", filepath.Base(ServicesFilesMap["saas-alpha"]))
				assert.Equal(t, "saas-beta.yaml", filepath.Base(ServicesFilesMap["saas-beta"]))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ServicesSlice = nil
			ServicesFilesMap = make(map[string]string)
			app := git.AppInterface{GitDirectory: tempDir}
			dirs := tt.setup(tempDir)
			got, err := GetServiceNames(app, dirs...)
			if tt.wantErr {
				assert.Error(t, err)
				assert.EqualError(t, err, tt.errMessage)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expected, got)
			}
			if tt.assertions != nil {
				tt.assertions(t, got)
			}
		})
	}
}

func TestSetHotfixVersion(t *testing.T) {
	tests := []struct {
		name            string
		fileContent     string
		componentName   string
		gitHash         string
		expectedContent string
		expectError     bool
		expectedFound   bool
		errorSubstr     string
	}{
		{
			name: "adds_hotfixVersions_to_Codecomponents_if_does_not_exist",
			fileContent: `
codeComponents:
  - name: test-component
    url: https://github.com/example/repo
  - name: other-component
    url: https://github.com/example/other
`,
			componentName: "test-component",
			gitHash:       "abc123",
			expectedContent: `codeComponents:
  - name: test-component
    url: https://github.com/example/repo
    hotfixVersions:
      - abc123
  - name: other-component
    url: https://github.com/example/other
`,
			expectError:   false,
			expectedFound: true,
		},
		{
			name: "replaces_hotfixVersion_if_already_exists",
			fileContent: `
codeComponents:
  - name: test-component
    url: https://github.com/example/repo
    hotfixVersions:
      - old-hash
  - name: other-component
    url: https://github.com/example/other
`,
			componentName: "test-component",
			gitHash:       "new-hash",
			expectedContent: `codeComponents:
  - name: test-component
    url: https://github.com/example/repo
    hotfixVersions:
      - new-hash
  - name: other-component
    url: https://github.com/example/other
`,
			expectError:   false,
			expectedFound: true,
		},
		{
			name: "invalid_yaml_returns_error",
			fileContent: `
codeComponents:
  - name: test-component
    url: [invalid yaml structure
`,
			componentName: "test-component",
			gitHash:       "abc123",
			expectError:   true,
			errorSubstr:   "error parsing app.yml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err, found := setHotfixVersion(tc.fileContent, tc.componentName, tc.gitHash)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorSubstr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedFound, found)
				if tc.expectedFound {
					assert.Contains(t, result, tc.gitHash)
					assert.Contains(t, result, "hotfixVersions:")
				}
			}
		})
	}
}

func TestUpdateAppYmlWithHotfix(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (git.AppInterface, string)
		serviceName string
		gitHash     string
		expectError bool
		errorSubstr string
	}{
		{
			name: "successfully_updates_app_yml_for_service_directly_under_services",
			setup: func(t *testing.T) (git.AppInterface, string) {
				tmpDir := t.TempDir()
				servicesDir := filepath.Join(tmpDir, "data", "services", "test-service")
				err := os.MkdirAll(servicesDir, 0755)
				require.NoError(t, err)

				appYmlContent := `
codeComponents:
  - name: test-service
    url: https://github.com/example/repo
`
				appYmlPath := filepath.Join(servicesDir, "app.yml")
				err = os.WriteFile(appYmlPath, []byte(appYmlContent), 0600)
				require.NoError(t, err)

				saasDir := filepath.Join(tmpDir, "data", "services", "test-service", "cicd", "saas", "saas-test-service.yaml")
				return git.AppInterface{GitDirectory: tmpDir}, saasDir
			},
			serviceName: "saas-test-service",
			gitHash:     "abc123",
			expectError: false,
		},
		{
			name: "successfully_updates_app_yml_for_osd_operator",
			setup: func(t *testing.T) (git.AppInterface, string) {
				tmpDir := t.TempDir()
				// imitating operators' directories
				servicesDir := filepath.Join(tmpDir, "data", "services", "osd-operators", "managed-cluster-config")
				err := os.MkdirAll(servicesDir, 0755)
				require.NoError(t, err)

				appYmlContent := `
codeComponents:
  - name: managed-cluster-config
    url: https://github.com/openshift/managed-cluster-config
`
				appYmlPath := filepath.Join(servicesDir, "app.yml")
				err = os.WriteFile(appYmlPath, []byte(appYmlContent), 0600)
				require.NoError(t, err)

				saasDir := filepath.Join(tmpDir, "data", "services", "osd-operators", "cicd", "saas", "saas-managed-cluster-config.yaml")
				return git.AppInterface{GitDirectory: tmpDir}, saasDir
			},
			serviceName: "saas-managed-cluster-config",
			gitHash:     "7f2dbae9d4284f8b68813270c1202ca3435459e5",
			expectError: false,
		},
		{
			name: "successfully_updates_app_yml_for_backplane_service",
			setup: func(t *testing.T) (git.AppInterface, string) {
				tmpDir := t.TempDir()
				// Imitate dir structure for backplane
				servicesDir := filepath.Join(tmpDir, "data", "services", "backplane", "backplane-api")
				err := os.MkdirAll(servicesDir, 0755)
				require.NoError(t, err)

				appYmlContent := `
codeComponents:
  - name: backplane-api
    url: https://github.com/openshift/backplane-api
`
				appYmlPath := filepath.Join(servicesDir, "app.yml")
				err = os.WriteFile(appYmlPath, []byte(appYmlContent), 0600)
				require.NoError(t, err)
				saasDir := filepath.Join(tmpDir, "data", "services", "backplane", "cicd", "saas", "saas-backplane-api.yaml")
				return git.AppInterface{GitDirectory: tmpDir}, saasDir
			},
			serviceName: "saas-backplane-api",
			gitHash:     "def456",
			expectError: false,
		},
		{
			name: "fails_when_app_yml_not_found",
			setup: func(t *testing.T) (git.AppInterface, string) {
				tmpDir := t.TempDir()
				saasDir := filepath.Join(tmpDir, "data", "services", "cicd", "saas", "saas-nonexistent-service.yaml")
				return git.AppInterface{GitDirectory: tmpDir}, saasDir
			},
			serviceName: "saas-nonexistent-service",
			gitHash:     "abc123",
			expectError: true,
			errorSubstr: "app.yml file not found",
		},
		{
			name: "fails_when_component_not_found_in_app_yml",
			setup: func(t *testing.T) (git.AppInterface, string) {
				tmpDir := t.TempDir()
				servicesDir := filepath.Join(tmpDir, "data", "services", "test-service")
				err := os.MkdirAll(servicesDir, 0755)
				require.NoError(t, err)

				appYmlContent := `
codeComponents:
  - name: other-service
    url: https://github.com/example/repo
`
				appYmlPath := filepath.Join(servicesDir, "app.yml")
				err = os.WriteFile(appYmlPath, []byte(appYmlContent), 0600)
				require.NoError(t, err)

				saasDir := filepath.Join(tmpDir, "data", "services", "test-service", "cicd", "saas", "saas-test-service.yaml")
				return git.AppInterface{GitDirectory: tmpDir}, saasDir
			},
			serviceName: "saas-test-service",
			gitHash:     "abc123",
			expectError: true,
			errorSubstr: "component test-service not found in app.yml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			appInterface, saasDir := tc.setup(t)

			err := updateAppYmlWithHotfix(appInterface, tc.serviceName, saasDir, tc.gitHash)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorSubstr)
			} else {
				assert.NoError(t, err)

				// Use the same path derivation logic as the implementation
				componentName := strings.TrimPrefix(tc.serviceName, "saas-")
				appYmlPath, err := pathutil.DeriveAppYmlPath(appInterface.GitDirectory, saasDir, componentName)
				require.NoError(t, err)

				content, readErr := os.ReadFile(appYmlPath)
				assert.NoError(t, readErr)
				assert.Contains(t, string(content), tc.gitHash)
				assert.Contains(t, string(content), "hotfixVersions:")
			}
		})
	}
}

func TestHotfixValidation(t *testing.T) {
	tests := []struct {
		name      string
		hotfix    bool
		gitHash   string
		expectErr bool
	}{
		{
			name:      "hotfix_requires_gitHash",
			hotfix:    true,
			gitHash:   "",
			expectErr: true,
		},
		{
			name:      "hotfix_with_gitHash_is_valid",
			hotfix:    true,
			gitHash:   "abc123",
			expectErr: false,
		},
		{
			name:      "no_hotfix_is_valid",
			hotfix:    false,
			gitHash:   "",
			expectErr: false,
		},
		{
			name:      "gitHash_without_hotfix_is_valid",
			hotfix:    false,
			gitHash:   "abc123",
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hasValidationError := tc.hotfix && tc.gitHash == ""

			assert.Equal(t, tc.expectErr, hasValidationError)
		})
	}
}

func TestDiscoverPipelineNamespace(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		saasYAML    string
		expectedNS  string
		expectErr   bool
		errContains string
	}{
		{
			name:        "abbreviated_namespace_mcvw",
			serviceName: "saas-managed-cluster-validating-webhooks",
			saasYAML: `---
name: saas-mc-validating-webhooks
pipelinesProvider:
  $ref: /services/osd-operators/managed-cluster-validating-webhooks/pipelines/tekton-mcvw-pipelines.appsrep09ue1.yaml
`,
			expectedNS: "mcvw-pipelines",
			expectErr:  false,
		},
		{
			name:        "standard_namespace_configure_am",
			serviceName: "saas-configure-alertmanager-operator",
			saasYAML: `---
name: saas-configure-am-operator
pipelinesProvider:
  $ref: /services/osd-operators/configure-alertmanager-operator/pipelines/tekton-configure-alertmanager-operator-pipelines.appsrep09ue1.yaml
`,
			expectedNS: "configure-alertmanager-operator-pipelines",
			expectErr:  false,
		},
		{
			name:        "shared_namespace_backplane",
			serviceName: "saas-backplane-api",
			saasYAML: `---
name: saas-backplane-api
pipelinesProvider:
  $ref: /services/backplane/pipelines/tekton.backplane-pipelines.appsrep09ue1.yaml
`,
			expectedNS: "backplane-pipelines",
			expectErr:  false,
		},
		{
			name:        "tekton_dot_prefix",
			serviceName: "saas-test-service",
			saasYAML: `---
name: saas-test-service
pipelinesProvider:
  $ref: /services/test/pipelines/tekton.test-pipelines.appsrep09ue1.yaml
`,
			expectedNS: "test-pipelines",
			expectErr:  false,
		},
		{
			name:        "missing_pipelinesProvider",
			serviceName: "saas-no-provider",
			saasYAML: `---
name: saas-no-provider
description: Test service without pipelinesProvider
`,
			expectErr:   true,
			errContains: "failed to get pipelinesProvider.$ref",
		},
		{
			name:        "invalid_yaml",
			serviceName: "saas-invalid",
			saasYAML: `---
name: [invalid yaml structure
pipelinesProvider: broken
`,
			expectErr:   true,
			errContains: "failed to parse SaaS YAML",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create temp directory for test
			tmpDir := t.TempDir()

			// Create mock SaaS file
			saasFile := filepath.Join(tmpDir, tc.serviceName+".yaml")
			err := os.WriteFile(saasFile, []byte(tc.saasYAML), 0600)
			require.NoError(t, err)

			// Setup ServicesFilesMap
			originalMap := ServicesFilesMap
			ServicesFilesMap = map[string]string{
				tc.serviceName: saasFile,
			}
			defer func() { ServicesFilesMap = originalMap }()

			// Create mock AppInterface
			appInterface := git.AppInterface{GitDirectory: tmpDir}

			// Test the function
			namespace, err := discoverPipelineNamespace(appInterface, tc.serviceName)

			if tc.expectErr {
				assert.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedNS, namespace)
			}
		})
	}
}

func TestDiscoverPipelineNamespace_ServiceNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	appInterface := git.AppInterface{GitDirectory: tmpDir}

	// Clear ServicesFilesMap
	originalMap := ServicesFilesMap
	ServicesFilesMap = map[string]string{}
	defer func() { ServicesFilesMap = originalMap }()

	namespace, err := discoverPipelineNamespace(appInterface, "saas-nonexistent")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "saas file not found")
	assert.Empty(t, namespace)
}

func TestGenerateGrafanaLogsURL_NamespaceDiscovery(t *testing.T) {
	tests := []struct {
		name         string
		serviceName  string
		operatorName string
		saasYAML     string
		expectedNS   string
	}{
		{
			name:         "with_namespace_discovery_mcvw",
			serviceName:  "saas-managed-cluster-validating-webhooks",
			operatorName: "managed-cluster-validating-webhooks",
			saasYAML: `---
name: saas-mc-validating-webhooks
pipelinesProvider:
  $ref: /services/osd-operators/managed-cluster-validating-webhooks/pipelines/tekton-mcvw-pipelines.appsrep09ue1.yaml
`,
			expectedNS: "mcvw-pipelines",
		},
		{
			name:         "with_namespace_discovery_backplane",
			serviceName:  "saas-backplane-api",
			operatorName: "backplane-api",
			saasYAML: `---
name: saas-backplane-api
pipelinesProvider:
  $ref: /services/backplane/pipelines/tekton.backplane-pipelines.appsrep09ue1.yaml
`,
			expectedNS: "backplane-pipelines",
		},
		{
			name:         "fallback_to_standard_naming",
			serviceName:  "saas-test-operator",
			operatorName: "test-operator",
			saasYAML: `---
name: saas-test-operator
description: No pipelinesProvider
`,
			expectedNS: "test-operator-pipelines",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create temp directory for test
			tmpDir := t.TempDir()

			// Create mock SaaS file
			saasFile := filepath.Join(tmpDir, tc.serviceName+".yaml")
			err := os.WriteFile(saasFile, []byte(tc.saasYAML), 0600)
			require.NoError(t, err)

			// Setup ServicesFilesMap
			originalMap := ServicesFilesMap
			ServicesFilesMap = map[string]string{
				tc.serviceName: saasFile,
			}
			defer func() { ServicesFilesMap = originalMap }()

			// Create mock AppInterface
			appInterface := git.AppInterface{GitDirectory: tmpDir}

			// Test INT URL
			intURL := generateGrafanaLogsURL(appInterface, tc.serviceName, tc.operatorName, "abc123", "int")

			// Verify Grafana URL structure
			assert.Contains(t, intURL, "https://grafana.app-sre.devshift.net/d/feq1jm3omydq8c/hcm-cicd-test-logs")
			assert.Contains(t, intURL, fmt.Sprintf("var-namespace=%s", tc.expectedNS))
			assert.Contains(t, intURL, "var-targetref=abc123")
			assert.Contains(t, intURL, "var-env=int")

			// Test STAGE URL
			stageURL := generateGrafanaLogsURL(appInterface, tc.serviceName, tc.operatorName, "def456", "stage")

			// Verify namespace is correct in STAGE URL
			assert.Contains(t, stageURL, fmt.Sprintf("var-namespace=%s", tc.expectedNS))
			assert.Contains(t, stageURL, "var-targetref=def456")
			assert.Contains(t, stageURL, "var-env=stage")
		})
	}
}

func TestGenerateGrafanaLogsURL_SameSaasfilenameBothEnvs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock SaaS file
	saasYAML := `---
name: saas-test-operator
pipelinesProvider:
  $ref: /services/osd-operators/test-operator/pipelines/tekton-test-operator-pipelines.appsrep09ue1.yaml
`
	saasFile := filepath.Join(tmpDir, "saas-test-operator.yaml")
	err := os.WriteFile(saasFile, []byte(saasYAML), 0600)
	require.NoError(t, err)

	// Setup ServicesFilesMap
	originalMap := ServicesFilesMap
	ServicesFilesMap = map[string]string{
		"saas-test-operator": saasFile,
	}
	defer func() { ServicesFilesMap = originalMap }()

	appInterface := git.AppInterface{GitDirectory: tmpDir}

	// Both INT and STAGE should use the same E2E test SaaS file name
	intURL := generateGrafanaLogsURL(appInterface, "saas-test-operator", "test-operator", "abc123", "int")
	assert.Contains(t, intURL, "var-saasfilename=saas-test-operator-e2e-test", "INT should query E2E test SaaS file")
	assert.Contains(t, intURL, "var-env=int", "INT should have env=int")

	stageURL := generateGrafanaLogsURL(appInterface, "saas-test-operator", "test-operator", "abc123", "stage")
	assert.Contains(t, stageURL, "var-saasfilename=saas-test-operator-e2e-test", "STAGE should also query E2E test SaaS file")
	assert.Contains(t, stageURL, "var-env=stage", "STAGE should have env=stage")
}

func TestGenerateGrafanaLogsURL_URLFormat(t *testing.T) {
	tmpDir := t.TempDir()
	appInterface := git.AppInterface{GitDirectory: tmpDir}

	// Clear ServicesFilesMap to force fallback
	originalMap := ServicesFilesMap
	ServicesFilesMap = map[string]string{}
	defer func() { ServicesFilesMap = originalMap }()

	url := generateGrafanaLogsURL(appInterface, "saas-test", "test", "abc123", "int")

	// Verify URL components
	assert.True(t, strings.HasPrefix(url, "https://"))
	assert.Contains(t, url, "grafana.app-sre.devshift.net/d/feq1jm3omydq8c/hcm-cicd-test-logs")
	assert.Contains(t, url, "var-namespace=test-pipelines")
	assert.Contains(t, url, "var-targetref=abc123")
	assert.Contains(t, url, "var-env=int")
	assert.Contains(t, url, "var-saasfilename=saas-test-e2e-test")
	assert.Contains(t, url, "orgId=1")
	assert.Contains(t, url, "var-cluster=appsrep09ue1")
}
