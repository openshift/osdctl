package dynatrace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommitSaasFile(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) (AppInterface, string)
		commitMessage string
		wantErr       bool
		expectedErr   string
	}{
		{
			name: "fails_when_file_does_not_exist",
			setup: func(t *testing.T) (AppInterface, string) {
				tmpDir := t.TempDir()
				_ = exec.Command("git", "init", tmpDir).Run()
				return AppInterface{GitDirectory: tmpDir}, "non-existent.yaml"
			},
			commitMessage: "commit non-existent file",
			wantErr:       true,
			expectedErr:   "failed to add file",
		},
		{
			name: "fails_when_not_a_git_repo",
			setup: func(t *testing.T) (AppInterface, string) {
				tmpDir := t.TempDir()
				file := filepath.Join(tmpDir, "saas.yaml")
				_ = os.WriteFile(file, []byte("content"), 0644)
				return AppInterface{GitDirectory: tmpDir}, file
			},
			commitMessage: "commit without git",
			wantErr:       true,
			expectedErr:   "failed to add file",
		},
		{
			name: "commits_file_successfully",
			setup: func(t *testing.T) (AppInterface, string) {
				tmpDir := t.TempDir()
				_ = exec.Command("git", "init", tmpDir).Run()

				// Set dummy git user config to avoid commit failure
				_ = exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run()
				_ = exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run()

				file := filepath.Join(tmpDir, "saas.yaml")
				_ = os.WriteFile(file, []byte("content"), 0644)
				return AppInterface{GitDirectory: tmpDir}, file
			},
			commitMessage: "valid commit",
			wantErr:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, file := tc.setup(t)
			err := app.CommitSaasFile(file, tc.commitMessage)

			if tc.wantErr {
				assert.Error(t, err)
				assert.ErrorContains(t, err, tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

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
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "init").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run())
				file := filepath.Join(tmpDir, "dummy.txt")
				assert.NoError(t, os.WriteFile(file, []byte("init"), 0644))
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "add", ".").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "commit", "-m", "initial commit").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "checkout", "-b", "master").Run())

				saasFile := filepath.Join(tmpDir, "test.yaml")
				err := os.WriteFile(saasFile, []byte("tag: old123"), 0644)
				assert.NoError(t, err)

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
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "init").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run())
				file := filepath.Join(tmpDir, "dummy.txt")
				assert.NoError(t, os.WriteFile(file, []byte("init"), 0644))
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "add", ".").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "commit", "-m", "initial commit").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "checkout", "-b", "master").Run())

				return AppInterface{GitDirectory: tmpDir}, filepath.Join(tmpDir, "nonexistent.yaml")
			},
			oldTag:      "old123",
			newTag:      "new456",
			expectedErr: "failed to read file",
			verify:      func(t *testing.T, _ string) {},
		},
		"no_change_when_old_tag_not_present": {
			setup: func(t *testing.T) (AppInterface, string) {
				tmpDir := t.TempDir()
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "init").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run())
				file := filepath.Join(tmpDir, "dummy.txt")
				assert.NoError(t, os.WriteFile(file, []byte("init"), 0644))
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "add", ".").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "commit", "-m", "initial commit").Run())
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "checkout", "-b", "master").Run())

				saasFile := filepath.Join(tmpDir, "test.yaml")
				err := os.WriteFile(saasFile, []byte("tag: somethingelse"), 0644)
				assert.NoError(t, err)

				return AppInterface{GitDirectory: tmpDir}, saasFile
			},
			oldTag:      "old123",
			newTag:      "new456",
			expectedErr: "",
			verify: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				assert.NoError(t, err)
				str := string(content)
				assert.Contains(t, str, "somethingelse")
				assert.NotContains(t, str, "new456")
			},
		},
		"error_when_git_checkout_fails": {
			setup: func(t *testing.T) (AppInterface, string) {
				tmpDir := t.TempDir()
				saasFile := filepath.Join(tmpDir, "test.yaml")
				_ = os.WriteFile(saasFile, []byte("tag: old123"), 0644)
				// no git init here to simulate failure
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
		wantHash      string
		wantRepo      string
		wantPath      string
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
			wantHash:      "abc123",
			wantRepo:      "https://github.com/test-org/test-repo.git",
			wantPath:      "some/path/to/file",
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
			wantErrSubstr: "service repo not found",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			hash, repo, path, err := GetCurrentGitHashFromAppInterface([]byte(tc.yamlContent), tc.serviceName)

			if tc.wantErrSubstr != "" {
				assert.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrSubstr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantHash, hash)
				assert.Equal(t, tc.wantRepo, repo)
				assert.Equal(t, tc.wantPath, path)
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
				return t.TempDir() // not initializing git
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
