package saas

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/osdctl/cmd/promote/git"
	"github.com/stretchr/testify/assert"
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
