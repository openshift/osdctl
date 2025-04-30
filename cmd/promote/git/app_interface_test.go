package git

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockExec struct {
	mock.Mock
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
			_ = os.WriteFile(file, []byte("dummy content"), 0644)

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

/*
func TestUpdatePackageTag(t *testing.T) {
	tests := map[string]struct {
		setup       func(t *testing.T) (AppInterface, string)
		oldTag      string
		newTag      string
		expectedErr string
		verify      func(t *testing.T, filePath string)
	}{
		"successfully_updates_tag": {
			setup: func(t *testing.T) (AppInterface, string) {
				tmpDir := t.TempDir()
				run := func(args ...string) {
					cmd := exec.Command("git", args...)
					cmd.Dir = tmpDir
					out, err := cmd.CombinedOutput()
					assert.NoError(t, err, "git %v failed: %s", args, string(out))
				}
				run("init")
				run("config", "user.name", "Test User")
				run("config", "user.email", "test@example.com")
				dummyFile := filepath.Join(tmpDir, "dummy.txt")
				assert.NoError(t, os.WriteFile(dummyFile, []byte("init"), 0644))
				run("add", ".")
				run("commit", "-m", "initial commit")
				cmd := exec.Command("git", "rev-parse", "--verify", "master")
				cmd.Dir = tmpDir
				if err := cmd.Run(); err != nil {
					run("checkout", "-b", "master")
				} else {
					run("checkout", "master")
				}
				saasFile := filepath.Join(tmpDir, "test.yaml")
				assert.NoError(t, os.WriteFile(saasFile, []byte("tag: old123"), 0644))
				run("add", ".")
				run("commit", "-m", "add saas file")
				run("checkout", "-b", "feature-branch")
				run("checkout", "master")
				return AppInterface{GitDirectory: tmpDir}, saasFile
			},

			oldTag:      "old123",
			newTag:      "new456",
			expectedErr: "",
			verify: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				assert.NoError(t, err)
				str := string(content)
				assert.Contains(t, str, "new456")
				assert.NotContains(t, str, "old123")
			},
		},
		"fails_when_file_does_not_exist": {
			setup: func(t *testing.T) (AppInterface, string) {
				tmpDir := t.TempDir()
				run := func(args ...string) {
					cmd := exec.Command("git", args...)
					cmd.Dir = tmpDir
					out, err := cmd.CombinedOutput()
					assert.NoError(t, err, "git %v failed: %s", args, string(out))
				}
				run("init")
				run("config", "user.name", "Test User")
				run("config", "user.email", "test@example.com")
				dummyFile := filepath.Join(tmpDir, "dummy.txt")
				assert.NoError(t, os.WriteFile(dummyFile, []byte("init"), 0644))
				run("add", ".")
				run("commit", "-m", "initial commit")

				// Ensure we're on master
				cmd := exec.Command("git", "rev-parse", "--verify", "master")
				cmd.Dir = tmpDir
				if err := cmd.Run(); err != nil {
					run("checkout", "-b", "master")
				} else {
					run("checkout", "master")
				}
				saasFile := filepath.Join(tmpDir, "test.yaml")
				assert.NoError(t, os.WriteFile(saasFile, []byte("tag: old123"), 0644))
				run("add", ".")
				run("commit", "-m", "add saas file")
				run("checkout", "-b", "feature-branch")
				run("checkout", "master")
				nonExistentFile := filepath.Join(tmpDir, "nonexistent.yaml")
				return AppInterface{GitDirectory: tmpDir}, nonExistentFile
			},
			oldTag:      "old123",
			newTag:      "new456",
			expectedErr: "failed to read file",
			verify:      func(t *testing.T, _ string) {},
		},
		"error_when_git_checkout_fails": {
			setup: func(t *testing.T) (AppInterface, string) {
				tmpDir := t.TempDir()
				saasFile := filepath.Join(tmpDir, "test.yaml")
				_ = os.WriteFile(saasFile, []byte("tag: old123"), 0644)
				return AppInterface{GitDirectory: tmpDir}, saasFile
			},
			oldTag:      "old123",
			newTag:      "new456",
			expectedErr: "failed to checkout master branch",
			verify:      func(t *testing.T, _ string) {},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			app, filePath := tc.setup(t)
			err := app.UpdatePackageTag(filePath, tc.oldTag, tc.newTag, "feature-branch")
			if tc.expectedErr != "" {
				assert.Error(t, err)
				assert.ErrorContains(t, err, tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}
			tc.verify(t, filePath)
		})
	}
}

func TestUpdateAppInterface(t *testing.T) {
	tests := map[string]struct {
		setup            func(t *testing.T) (AppInterface, string)
		serviceName      string
		currentGitHash   string
		promotionGitHash string
		branchName       string
		expectedErr      string
		verify           func(t *testing.T, filePath string)
	}{
		"successfully_updates_hash": {
			setup: func(t *testing.T) (AppInterface, string) {
				tmpDir := t.TempDir()
				saasFilePath := filepath.Join(tmpDir, "test.yaml")
				content := fmt.Sprintf("ref: %s\n", "abc123")
				_ = os.WriteFile(saasFilePath, []byte(content), 0644)
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "init").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "add", ".").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "commit", "-m", "initial commit").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "branch", "-m", "master").Run())
				return AppInterface{GitDirectory: tmpDir}, saasFilePath
			},
			serviceName:      "test-component",
			currentGitHash:   "abc123",
			promotionGitHash: "def456",
			branchName:       "feature-branch",
			expectedErr:      "",
			verify: func(t *testing.T, filePath string) {
				updatedContent, readErr := os.ReadFile(filePath)
				assert.NoError(t, readErr)
				assert.Contains(t, string(updatedContent), "def456")
				assert.NotContains(t, string(updatedContent), "abc123")
			},
		},
		"fails_when_git_checkout_fails": {
			setup: func(t *testing.T) (AppInterface, string) {
				tmpDir := t.TempDir()
				saasFile := filepath.Join(tmpDir, "test.yaml")
				_ = os.WriteFile(saasFile, []byte("sha: abcdef"), 0644)
				return AppInterface{GitDirectory: tmpDir}, saasFile
			},
			serviceName:      "test-service",
			currentGitHash:   "abcdef",
			promotionGitHash: "123456",
			branchName:       "feature-fail-checkout",
			expectedErr:      "failed to checkout master branch",
			verify:           func(t *testing.T, _ string) {},
		},
		"fails_when_file_does_not_exist": {
			setup: func(t *testing.T) (AppInterface, string) {
				tmpDir := t.TempDir()
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "init").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run())
				file := filepath.Join(tmpDir, "dummy.txt")
				assert.NoError(t, os.WriteFile(file, []byte("init"), 0644))
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "add", ".").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "commit", "-m", "initial commit").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "branch", "-m", "master").Run())
				return AppInterface{GitDirectory: tmpDir}, filepath.Join(tmpDir, "nonexistent.yaml")
			},
			serviceName:      "test-service",
			currentGitHash:   "abcdef",
			promotionGitHash: "123456",
			branchName:       "feature-fail-read",
			expectedErr:      "failed to read file",
			verify:           func(t *testing.T, _ string) {},
		},
		"fails_when_git_directory_does_not_exist": {
			setup: func(t *testing.T) (AppInterface, string) {
				nonExistentDir := filepath.Join(os.TempDir(), "nonexistent-dir")
				return AppInterface{GitDirectory: nonExistentDir}, filepath.Join(nonExistentDir, "test.yaml")
			},
			serviceName:      "test-service",
			currentGitHash:   "abcdef",
			promotionGitHash: "123456",
			branchName:       "feature-no-dir",
			expectedErr:      "failed to checkout master branch",
			verify:           func(t *testing.T, _ string) {},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			app, filePath := tc.setup(t)
			err := app.UpdateAppInterface(tc.serviceName, filePath, tc.currentGitHash, tc.promotionGitHash, tc.branchName)

			if tc.expectedErr != "" {
				assert.Error(t, err)
				assert.ErrorContains(t, err, tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}

			tc.verify(t, filePath)
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
        name: production-hivep`,
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
        name: production-hivep`,
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
        name: production-special`,
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
        name: production-other`,
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
        ref: hash321`,
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
        ref: hash999`,
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
        name: staging`,
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
        name: staging`,
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
        name: staging`,
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

func TestCheckAppInterfaceCheckout(t *testing.T) {
	tests := map[string]struct {
		setupDir      func(t *testing.T) string
		expectedError string
	}{
		"valid app-interface remote": {
			setupDir: func(t *testing.T) string {
				dir := t.TempDir()
				assert.NoError(t, exec.Command("git", "-C", dir, "init").Run())
				assert.NoError(t, exec.Command("git", "-C", dir, "remote", "add", "origin", "git@gitlab.cee.redhat.com/app-interface.git").Run())
				return dir
			},
		},
		"non app-interface remote": {
			setupDir: func(t *testing.T) string {
				dir := t.TempDir()
				assert.NoError(t, exec.Command("git", "-C", dir, "init").Run())
				assert.NoError(t, exec.Command("git", "-C", dir, "remote", "add", "origin", "git@github.com/test/repo.git").Run())
				return dir
			},
			expectedError: "not running in checkout of app-interface",
		},
		"non-git directory": {
			setupDir: func(t *testing.T) string {
				return t.TempDir()
			},
			expectedError: "error executing 'git remote -v'",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := tc.setupDir(t)
			err := checkAppInterfaceCheckout(dir)

			if tc.expectedError != "" {
				assert.Error(t, err)
				assert.ErrorContains(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
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
			err := os.WriteFile(saasFilePath, []byte(tt.yamlData), 0644)
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
*/
