package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openshift/osdctl/cmd/promote/iexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockExec struct {
	mock.Mock
	iexec.IExec
}

func (m *MockExec) Run(dir string, name string, args ...string) error {
	argsList := m.Called(dir, name, args)
	return argsList.Error(0)
}

func (m *MockExec) Output(dir, cmd string, args ...string) (string, error) {
	argsList := m.Called(dir, cmd, args)
	return argsList.String(0), argsList.Error(1)
}

func TestCommitSaasFile(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(m *MockExec, dir, file, commitMsg string)
		commitMessage string
		wantErr       bool
		expectedErr   string
	}{
		{
			name: "fails_when_file_does_not_exist",
			setupMock: func(m *MockExec, dir, file, _ string) {
				m.On("Run", dir, "git", []string{"add", file}).Return(errors.New("file not found"))
			},
			commitMessage: "commit non-existent file",
			wantErr:       true,
			expectedErr:   "failed to add file",
		},
		{
			name: "fails_when_not_a_git_repo",
			setupMock: func(m *MockExec, dir, file, _ string) {
				m.On("Run", dir, "git", []string{"add", file}).Return(nil)
				m.On("Run", dir, "git", []string{"commit", "-m", "commit without git"}).Return(errors.New("not a git repo"))
			},
			commitMessage: "commit without git",
			wantErr:       true,
			expectedErr:   "failed to commit changes",
		},
		{
			name: "commits_file_successfully",
			setupMock: func(m *MockExec, dir, file, msg string) {
				m.On("Run", dir, "git", []string{"add", file}).Return(nil)
				m.On("Run", dir, "git", []string{"commit", "-m", msg}).Return(nil)
			},
			commitMessage: "valid commit",
			wantErr:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			file := filepath.Join(tmpDir, "saas.yaml")
			_ = os.WriteFile(file, []byte("dummy content"), 0o600)

			mockExec := new(MockExec)
			tc.setupMock(mockExec, tmpDir, file, tc.commitMessage)

			app := AppInterface{
				GitDirectory: tmpDir,
				GitExecutor:  mockExec,
			}

			err := app.CommitSaasFile(file, tc.commitMessage)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}

			mockExec.AssertExpectations(t)
		})
	}
}

func TestGetCurrentGitHashFromAppInterface(t *testing.T) {
	tests := map[string]struct {
		yamlContent   string
		serviceName   string
		namespaceRef  string
		wantHash      string
		wantRepo      string
		wantErrSubstr string
	}{
		"successfully_extracts_git_hash_repo_path": {
			yamlContent: `
resourceTemplates:
  - name: app-interface
    url: https://github.com/test-org/test-repo.git
    path: some/path/to/file
    targets:
      - namespace:
          $ref: /services/test/namespace/hivep.yml
        ref: abc123
        name: production-hivep
`,
			serviceName:   "test-service",
			namespaceRef:  "",
			wantHash:      "abc123",
			wantRepo:      "https://github.com/test-org/test-repo.git",
			wantErrSubstr: "",
		},
		"fails_when_repo_is_missing": {
			yamlContent: `
resourceTemplates:
  - name: app-interface
    targets:
      - namespace:
          $ref: /services/test/namespace/hivep.yml
        ref: def456
        name: production-hivep
`,
			serviceName:   "test-service",
			namespaceRef:  "",
			wantErrSubstr: "service repo not found",
		},
		"successfully_extracts_hash_with_namespaceRef": {
			yamlContent: `
resourceTemplates:
  - name: app-interface
    url: https://github.com/test-org/test-repo.git
    targets:
      - namespace:
          $ref: /services/special/namespace/custom-namespace.yml
        ref: specialhash
        name: production-special
`,
			serviceName:   "test-service",
			namespaceRef:  "custom-namespace.yml",
			wantHash:      "specialhash",
			wantRepo:      "https://github.com/test-org/test-repo.git",
			wantErrSubstr: "",
		},
		"fails_when_no_target_matches_namespaceRef": {
			yamlContent: `
resourceTemplates:
  - name: app-interface
    url: https://github.com/test-org/test-repo.git
    targets:
      - namespace:
          $ref: /services/other/namespace/another.yml
        ref: somehash
        name: production-other
`,
			serviceName:   "test-service",
			namespaceRef:  "nonexistent-namespace.yml",
			wantErrSubstr: "production namespace not found",
		},
		"successfully_extracts_hash_when_namespaceRef_matches_for_db": {
			yamlContent: `
name: saas-configuration-anomaly-detection-db
resourceTemplates:
  - name: app-interface
    url: https://github.com/test-org/db-repo.git
    targets:
      - namespace:
          $ref: /services/production/namespace/app-sre-observability-production-int.yml
        ref: hash321
`,
			serviceName:   "saas-configuration-anomaly-detection-db",
			namespaceRef:  "",
			wantHash:      "hash321",
			wantRepo:      "https://github.com/test-org/db-repo.git",
			wantErrSubstr: "",
		},
		"successfully_extracts_git_hash_for_configuration_anomaly_detection": {
			yamlContent: `
name: saas-configuration-anomaly-detection-service
resourceTemplates:
  - name: app-interface
    url: https://github.com/test-org/obs-repo.git
    targets:
      - namespace:
          $ref: configuration-anomaly-detection-production
        ref: hash999
`,
			serviceName:   "saas-configuration-anomaly-detection-service",
			namespaceRef:  "",
			wantHash:      "hash999",
			wantRepo:      "https://github.com/test-org/obs-repo.git",
			wantErrSubstr: "",
		},
		"successfully_extracts_hash_for_rhobs_rules_and_dashboards": {
			yamlContent: `
name: rhobs-rules-and-dashboards-production
resourceTemplates:
  - name: app-interface
    url: https://github.com/org/rhobs-rules.git
    targets:
      - namespace:
          $ref: /services/prod/namespace/rhobs-production.yml
        ref: rhobs789
        name: production
      - namespace:
          $ref: /services/prod/namespace/staging.yml
        ref: rhobs012
        name: staging
`,
			serviceName:   "rhobs-rules-and-dashboards-production",
			namespaceRef:  "",
			wantHash:      "rhobs789",
			wantRepo:      "https://github.com/org/rhobs-rules.git",
			wantErrSubstr: "",
		},
		"successfully_extracts_hash_for_saas_backplane_api": {
			yamlContent: `
name: saas-backplane-api
resourceTemplates:
  - name: app-interface
    url: https://github.com/test-org/backplane.git
    targets:
      - namespace:
          $ref: /services/prod/namespace/backplanep-production.yml
        ref: backplane123
        name: production
      - namespace:
          $ref: /services/prod/namespace/backplanep-staging.yml
        ref: backplane456
        name: staging
`,
			serviceName:   "saas-backplane-api",
			namespaceRef:  "backplanep",
			wantHash:      "backplane123",
			wantRepo:      "https://github.com/test-org/backplane.git",
			wantErrSubstr: "",
		},
		"successfully_extracts_hash_when_namespace_ref_contains_backplanep": {
			yamlContent: `
name: saas-backplane-api
resourceTemplates:
  - name: app-interface
    url: https://github.com/test-org/backplane.git
    targets:
      - namespace:
          $ref: /services/prod/namespace/backplanep-production.yml
        ref: backplane123
        name: production
      - namespace:
          $ref: /services/prod/namespace/othernamespace.yml
        ref: otherhash
        name: staging
`,
			serviceName:   "saas-backplane-api",
			namespaceRef:  "backplanep",
			wantHash:      "backplane123",
			wantRepo:      "https://github.com/test-org/backplane.git",
			wantErrSubstr: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			hash, repo, err := GetCurrentGitHashFromAppInterface([]byte(tc.yamlContent), tc.serviceName, tc.namespaceRef)

			if tc.wantErrSubstr != "" {
				assert.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrSubstr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantHash, hash)
				assert.Equal(t, tc.wantRepo, repo)
			}
		})
	}
}

func TestGetCurrentPackageTagFromAppInterface(t *testing.T) {
	tests := []struct {
		name        string
		yamlData    string
		expected    string
		expectError bool
		errorSubstr string
	}{
		{
			name: "valid service with matching hivep ref",
			yamlData: `
name: my-service
resourceTemplates:
  - name: my-service-package-template
    targets:
      - namespace:
          $ref: /services/hivep/production/some-ns
        parameters:
          PACKAGE_TAG: v1.2.3
`,
			expected:    "v1.2.3",
			expectError: false,
		},
		{
			name: "service with configuration-anomaly-detection name",
			yamlData: `
name: configuration-anomaly-detection-service
resourceTemplates: []
`,
			expectError: true,
			errorSubstr: "cannot promote package for configuration-anomaly-detection",
		},
		{
			name: "service with rhobs-rules-and-dashboards name",
			yamlData: `
name: rhobs-rules-and-dashboards-main
resourceTemplates: []
`,
			expectError: true,
			errorSubstr: "cannot promote package for rhobs-rules-and-dashboards",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			saasFilePath := filepath.Join(tmpDir, "saas.yaml")
			err := os.WriteFile(saasFilePath, []byte(tt.yamlData), 0o600)
			require.NoError(t, err)

			actual, err := GetCurrentPackageTagFromAppInterface(saasFilePath)

			if tt.expectError {
				require.Error(t, err)
				require.True(t, strings.Contains(err.Error(), tt.errorSubstr), "unexpected error: %v", err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, actual)
			}
		})
	}
}

func TestUpdatePackageTag(t *testing.T) {
	type testCase struct {
		name         string
		setup        func(t *testing.T) (AppInterface, string)
		expectedErr  string
		expectedFile string
	}

	tests := []testCase{
		{
			name: "successfully_updates_tag_in_file",
			setup: func(t *testing.T) (AppInterface, string) {
				mockExec := new(MockExec)
				tmpDir := t.TempDir()
				saasFile := filepath.Join(tmpDir, "test.yaml")

				_ = os.WriteFile(saasFile, []byte("tag: old123"), 0o600)

				mockExec.On("Run", tmpDir, "git", []string{"checkout", "master"}).Return(nil).Once()
				mockExec.On("Run", tmpDir, "git", []string{"branch", "-D", "feature-branch"}).Return(errors.New("branch does not exist")).Once()

				return AppInterface{
					GitDirectory: tmpDir,
					GitExecutor:  mockExec,
				}, saasFile
			},
			expectedErr:  "",
			expectedFile: "tag: new456",
		},
		{
			name: "fails_git_checkout",
			setup: func(t *testing.T) (AppInterface, string) {
				mockExec := new(MockExec)
				tmpDir := t.TempDir()
				saasFile := filepath.Join(tmpDir, "test.yaml")

				_ = os.WriteFile(saasFile, []byte("tag: old123"), 0o600)

				mockExec.On("Run", tmpDir, "git", []string{"checkout", "master"}).Return(errors.New("checkout failed")).Once()

				return AppInterface{
					GitDirectory: tmpDir,
					GitExecutor:  mockExec,
				}, saasFile
			},
			expectedErr:  "failed to checkout master branch",
			expectedFile: "tag: old123",
		},
		{
			name: "fails_reading_file",
			setup: func(t *testing.T) (AppInterface, string) {
				mockExec := new(MockExec)
				tmpDir := t.TempDir()
				saasFile := filepath.Join(tmpDir, "nonexistent.yaml")

				mockExec.On("Run", tmpDir, "git", []string{"checkout", "master"}).Return(nil).Once()
				mockExec.On("Run", tmpDir, "git", []string{"branch", "-D", "feature-branch"}).Return(nil).Once()

				return AppInterface{
					GitDirectory: tmpDir,
					GitExecutor:  mockExec,
				}, saasFile
			},
			expectedErr:  "failed to read file",
			expectedFile: "",
		},
		{
			name: "fails_writing_file",
			setup: func(t *testing.T) (AppInterface, string) {
				mockExec := new(MockExec)
				tmpDir := t.TempDir()
				saasFile := filepath.Join(tmpDir, "readonly.yaml")

				_ = os.WriteFile(saasFile, []byte("tag: old123"), 0o400)

				mockExec.On("Run", tmpDir, "git", []string{"checkout", "master"}).Return(nil).Once()
				mockExec.On("Run", tmpDir, "git", []string{"branch", "-D", "feature-branch"}).Return(nil).Once()

				return AppInterface{
					GitDirectory: tmpDir,
					GitExecutor:  mockExec,
				}, saasFile
			},
			expectedErr:  "failed to write to file",
			expectedFile: "tag: old123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, saasFile := tc.setup(t)

			err := app.UpdatePackageTag(saasFile, "old123", "new456", "feature-branch")

			if tc.expectedErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}

			if tc.expectedFile != "" {
				data, readErr := os.ReadFile(saasFile)
				assert.NoError(t, readErr)
				assert.Equal(t, tc.expectedFile, string(data))
			}

			if mockExec, ok := app.GitExecutor.(*MockExec); ok {
				mockExec.AssertExpectations(t)
			}
		})
	}
}

func TestUpdateAppInterface(t *testing.T) {
	tests := map[string]struct {
		setup              func(t *testing.T) (AppInterface, string)
		service_name       string
		current_git_hash   string
		promotion_git_hash string
		branch_name        string
		expected_err       string
		verify             func(t *testing.T, saas_file string)
	}{
		"success_case": {
			setup: func(t *testing.T) (AppInterface, string) {
				tmp_dir := t.TempDir()
				saas_file := filepath.Join(tmp_dir, "test.yaml")

				yaml_content := `
resourceTemplates:
  - name: "template1"
    targets:
      - name: "target-canary"
        ref: "currentGitHash"
      - name: "target-prod"
        ref: "currentGitHash"
  - name: "template2"
    targets:
      - name: "target-canary"
        ref: "currentGitHash"
`
				if err := os.WriteFile(saas_file, []byte(yaml_content), 0o600); err != nil {
					t.Fatalf("failed to write saas file: %v", err)
				}

				mock_exec := new(MockExec)
				mock_exec.On("Run", "/path/to/git/dir", "git", []string{"checkout", "master"}).Return(nil).Once()
				mock_exec.On("Run", "/path/to/git/dir", "git", []string{"branch", "-D", "feature-branch"}).Return(nil).Once()
				mock_exec.On("Run", "/path/to/git/dir", "git", []string{"checkout", "-b", "feature-branch", "master"}).Return(nil).Once()

				return AppInterface{
					GitDirectory: "/path/to/git/dir",
					GitExecutor:  mock_exec,
				}, saas_file
			},
			service_name:       "test-service",
			current_git_hash:   "currentGitHash",
			promotion_git_hash: "promotionGitHash",
			branch_name:        "feature-branch",
			expected_err:       "",
			verify:             func(t *testing.T, _ string) {},
		},
		"git_checkout_error": {
			setup: func(t *testing.T) (AppInterface, string) {
				tmp_dir := t.TempDir()
				saas_file := filepath.Join(tmp_dir, "test.yaml")

				yaml_content := `
resourceTemplates:
  - name: "template1"
    targets:
      - name: "target-canary"
        ref: "currentGitHash"
      - name: "target-prod"
        ref: "currentGitHash"
`
				if err := os.WriteFile(saas_file, []byte(yaml_content), 0o600); err != nil {
					t.Fatalf("failed to write saas file: %v", err)
				}

				mock_exec := new(MockExec)
				mock_exec.On("Run", "/path/to/git/dir", "git", []string{"checkout", "master"}).Return(errors.New("failed to checkout master branch")).Once()

				return AppInterface{
					GitDirectory: "/path/to/git/dir",
					GitExecutor:  mock_exec,
				}, saas_file
			},
			service_name:       "test-service",
			current_git_hash:   "currentGitHash",
			promotion_git_hash: "promotionGitHash",
			branch_name:        "feature-error",
			expected_err:       "failed to checkout master branch",
			verify:             func(t *testing.T, _ string) {},
		},
		"fails_when_git_directory_does_not_exist": {
			setup: func(t *testing.T) (AppInterface, string) {
				non_existent_dir := filepath.Join(os.TempDir(), "nonexistent-dir")
				mock_exec := new(MockExec)
				// Simulate failure on git checkout due to missing directory
				mock_exec.On("Run", non_existent_dir, "git", []string{"checkout", "master"}).
					Return(errors.New("failed to checkout master branch")).Once()

				return AppInterface{
					GitDirectory: non_existent_dir,
					GitExecutor:  mock_exec,
				}, filepath.Join(non_existent_dir, "test.yaml")
			},
			service_name:       "test-service",
			current_git_hash:   "abcdef",
			promotion_git_hash: "123456",
			branch_name:        "feature-no-dir",
			expected_err:       "failed to checkout master branch",
			verify:             func(t *testing.T, _ string) {},
		},
		"fails_when_file_does_not_exist": {
			setup: func(t *testing.T) (AppInterface, string) {
				tmpDir := t.TempDir()
				saasFilePath := filepath.Join(tmpDir, "nonexistent.yaml") // file intentionally does not exist

				mockExec := new(MockExec)
				mockExec.On("Run", tmpDir, "git", []string{"checkout", "master"}).Return(nil).Once()
				mockExec.On("Run", tmpDir, "git", []string{"branch", "-D", "feature-fail-read"}).Return(nil).Once()
				mockExec.On("Run", tmpDir, "git", []string{"checkout", "-b", "feature-fail-read", "master"}).Return(nil).Once()

				return AppInterface{
					GitDirectory: tmpDir,
					GitExecutor:  mockExec,
				}, saasFilePath
			},
			service_name:       "test-service",
			current_git_hash:   "abcdef",
			promotion_git_hash: "123456",
			branch_name:        "feature-fail-read",
			expected_err:       "failed to read file",
			verify:             func(t *testing.T, _ string) {},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			app, saas_file := tt.setup(t)

			err := app.UpdateAppInterface(tt.service_name, saas_file, tt.current_git_hash, tt.promotion_git_hash, tt.branch_name, false)
			if tt.expected_err != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expected_err)
			} else {
				require.NoError(t, err)
			}

			if app.GitExecutor != nil {
				app.GitExecutor.(*MockExec).AssertExpectations(t)
			}

			tt.verify(t, saas_file)
		})
	}
}

func TestCommitSaasAndAppYmlFile(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(m *MockExec, dir, saasFile, serviceName, commitMsg string)
		serviceName string
		commitMsg   string
		wantErr     bool
		expectedErr string
	}{
		{
			name: "commits_saas_and_app_yml_successfully",
			setupMock: func(m *MockExec, dir, saasFile, serviceName, commitMsg string) {
				appYmlPath := filepath.Join(dir, "data", "services", "test-service", "app.yml")
				m.On("Run", dir, "git", []string{"add", saasFile}).Return(nil)
				m.On("Run", dir, "git", []string{"add", appYmlPath}).Return(nil)
				m.On("Run", dir, "git", []string{"commit", "-m", commitMsg}).Return(nil)
			},
			serviceName: "saas-test-service",
			commitMsg:   "test hotfix commit",
			wantErr:     false,
		},
		{
			name: "commits_osd_operator_saas_and_app_yml_successfully",
			setupMock: func(m *MockExec, dir, saasFile, serviceName, commitMsg string) {
				//osd-operators have the app.yml under osd-operators/<name>/app.yml
				appYmlPath := filepath.Join(dir, "data", "services", "osd-operators", "managed-cluster-config", "app.yml")
				m.On("Run", dir, "git", []string{"add", saasFile}).Return(nil)
				m.On("Run", dir, "git", []string{"add", appYmlPath}).Return(nil)
				m.On("Run", dir, "git", []string{"commit", "-m", commitMsg}).Return(nil)
			},
			serviceName: "saas-managed-cluster-config",
			commitMsg:   "test hotfix commit for OSD operator",
			wantErr:     false,
		},
		{
			name: "fails_when_saas_file_add_fails",
			setupMock: func(m *MockExec, dir, saasFile, serviceName, commitMsg string) {
				m.On("Run", dir, "git", []string{"add", saasFile}).Return(errors.New("saas file not found"))
			},
			serviceName: "saas-test-service",
			commitMsg:   "test commit",
			wantErr:     true,
			expectedErr: "failed to add file",
		},
		{
			name: "fails_when_app_yml_add_fails",
			setupMock: func(m *MockExec, dir, saasFile, serviceName, commitMsg string) {
				appYmlPath := filepath.Join(dir, "data", "services", "test-service", "app.yml")
				m.On("Run", dir, "git", []string{"add", saasFile}).Return(nil)
				m.On("Run", dir, "git", []string{"add", appYmlPath}).Return(errors.New("app.yml not found"))
			},
			serviceName: "saas-test-service",
			commitMsg:   "test commit",
			wantErr:     true,
			expectedErr: "failed to add file",
		},
		{
			name: "fails_when_commit_fails",
			setupMock: func(m *MockExec, dir, saasFile, serviceName, commitMsg string) {
				appYmlPath := filepath.Join(dir, "data", "services", "test-service", "app.yml")
				m.On("Run", dir, "git", []string{"add", saasFile}).Return(nil)
				m.On("Run", dir, "git", []string{"add", appYmlPath}).Return(nil)
				m.On("Run", dir, "git", []string{"commit", "-m", commitMsg}).Return(errors.New("commit failed"))
			},
			serviceName: "saas-test-service",
			commitMsg:   "test commit",
			wantErr:     true,
			expectedErr: "failed to commit changes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var saasFile string
			if tc.serviceName == "saas-managed-cluster-config" {
				// osd-operator path structure
				saasFile = filepath.Join(tmpDir, "data", "services", "osd-operators", "cicd", "saas", "saas-managed-cluster-config.yaml")
			} else {
				// Regular path structure
				saasFile = filepath.Join(tmpDir, "data", "services", "test-service", "cicd", "saas", "saas-test-service.yaml")
			}
			_ = os.MkdirAll(filepath.Dir(saasFile), 0o755)
			_ = os.WriteFile(saasFile, []byte("dummy content"), 0o600)

			mockExec := new(MockExec)
			tc.setupMock(mockExec, tmpDir, saasFile, tc.serviceName, tc.commitMsg)

			app := AppInterface{
				GitDirectory: tmpDir,
				GitExecutor:  mockExec,
			}

			err := app.CommitSaasAndAppYmlFile(saasFile, tc.serviceName, tc.commitMsg)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}

			mockExec.AssertExpectations(t)
		})
	}
}

func TestCheckAppInterfaceCheckout(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(m *MockExec, dir string)
		expectedErr string
		wantErr     bool
	}{
		{
			name: "succeeds_when_in_app_interface_checkout",
			setupMock: func(m *MockExec, dir string) {
				m.On("Output", dir, "git", []string{"remote", "-v"}).Return("origin  git@gitlab.cee.redhat.com:app-interface/repo.git (fetch)", nil)
			},
			expectedErr: "",
			wantErr:     false,
		},
		{
			name: "fails_when_not_in_app_interface_checkout",
			setupMock: func(m *MockExec, dir string) {
				m.On("Output", dir, "git", []string{"remote", "-v"}).Return("origin  git@gitlab.cee.redhat.com:some-other-repo.git (fetch)", fmt.Errorf("error"))
			},
			expectedErr: "error executing 'git remote -v",
			wantErr:     true,
		},
		{
			name: "fails_when_git_command_fails",
			setupMock: func(m *MockExec, dir string) {
				m.On("Output", dir, "git", []string{"remote", "-v"}).Return("", fmt.Errorf("git command failed"))
			},
			expectedErr: "error executing 'git remote -v': git command failed",
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use a temporary directory as the git directory
			tmpDir := t.TempDir()

			// Create the mock GitExecutor and set up the mock behavior
			mockGitExecutor := new(MockExec)
			tc.setupMock(mockGitExecutor, tmpDir)

			// Create the AppInterface with the mocked GitExecutor
			appInterface := &AppInterface{
				GitDirectory: tmpDir,
				GitExecutor:  mockGitExecutor,
			}

			// Run the checkAppInterfaceCheckout method
			err := appInterface.checkAppInterfaceCheckout()

			// Check if the error is as expected
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErr)
			} else {
				require.NoError(t, err)
			}

			// Assert the mock expectations
			mockGitExecutor.AssertExpectations(t)
		})
	}
}
