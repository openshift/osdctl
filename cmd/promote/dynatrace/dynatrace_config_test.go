package dynatrace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitFiles(t *testing.T) {
	tests := map[string]struct {
		setup       func(t *testing.T) DynatraceConfig
		commitMsg   string
		expectError bool
	}{
		"successfully_commits_file": {
			setup: func(t *testing.T) DynatraceConfig {
				tmpDir := t.TempDir()
				require.NoError(t, exec.Command("git", "init", tmpDir).Run())
				require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run())
				require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run())
				filePath := filepath.Join(tmpDir, "dummy.txt")
				require.NoError(t, os.WriteFile(filePath, []byte("dummy content"), 0644))
				return DynatraceConfig{GitDirectory: tmpDir}
			},
			commitMsg:   "initial commit",
			expectError: false,
		},
		"fails_when_git_not_initialized": {
			setup: func(t *testing.T) DynatraceConfig {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "dummy.txt")
				require.NoError(t, os.WriteFile(filePath, []byte("content"), 0644))
				return DynatraceConfig{GitDirectory: tmpDir}
			},
			commitMsg:   "should fail",
			expectError: true,
		},
		"fails_when_git_add_fails": {
			setup: func(t *testing.T) DynatraceConfig {
				tmpDir := t.TempDir()
				require.NoError(t, exec.Command("git", "init", tmpDir).Run())
				require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run())
				require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run())
				// Change permissions to break git add
				require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "unreadable"), 0000))

				return DynatraceConfig{GitDirectory: tmpDir}
			},
			commitMsg:   "will fail on add",
			expectError: true,
		},
		"fails_when_git_commit_fails": {
			setup: func(t *testing.T) DynatraceConfig {
				tmpDir := t.TempDir()
				require.NoError(t, exec.Command("git", "init", tmpDir).Run())
				require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run())
				require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run())
				return DynatraceConfig{GitDirectory: tmpDir}
			},
			commitMsg:   "empty commit",
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := tc.setup(t)
			err := cfg.commitFiles(tc.commitMsg)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				out, err := exec.Command("git", "-C", cfg.GitDirectory, "log", "--oneline").Output()
				require.NoError(t, err)
				assert.Contains(t, string(out), tc.commitMsg)
			}
		})
	}
}

func Test_UpdateDynatraceConfig(t *testing.T) {
	tests := map[string]struct {
		setup       func(t *testing.T) DynatraceConfig
		expectError bool
	}{
		"successfully_updates_branch": {
			setup: func(t *testing.T) DynatraceConfig {
				tmpDir := t.TempDir()
				require.NoError(t, exec.Command("git", "init", "--initial-branch=main", tmpDir).Run())
				require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.name", "Test").Run())
				require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run())
				file := filepath.Join(tmpDir, "dummy.txt")
				require.NoError(t, os.WriteFile(file, []byte("data"), 0644))
				require.NoError(t, exec.Command("git", "-C", tmpDir, "add", ".").Run())
				require.NoError(t, exec.Command("git", "-C", tmpDir, "commit", "-m", "init").Run())
				return DynatraceConfig{GitDirectory: tmpDir}
			},
			expectError: false,
		},
		"fails_when_checkout_main_fails": {
			setup: func(t *testing.T) DynatraceConfig {
				tmpDir := t.TempDir()
				return DynatraceConfig{GitDirectory: tmpDir}
			},
			expectError: true,
		},
		"ignores_delete_branch_failure": {
			setup: func(t *testing.T) DynatraceConfig {
				tmpDir := t.TempDir()
				require.NoError(t, exec.Command("git", "init", "--initial-branch=main", tmpDir).Run())
				require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.name", "Test").Run())
				require.NoError(t, exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run())
				file := filepath.Join(tmpDir, "dummy.txt")
				require.NoError(t, os.WriteFile(file, []byte("x"), 0644))
				require.NoError(t, exec.Command("git", "-C", tmpDir, "add", ".").Run())
				require.NoError(t, exec.Command("git", "-C", tmpDir, "commit", "-m", "init").Run())
				return DynatraceConfig{GitDirectory: tmpDir}
			},
			expectError: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := tc.setup(t)
			err := cfg.UpdateDynatraceConfig("component", "promoHash", "already_exists")
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_checkDynatraceConfigCheckout(t *testing.T) {
	tests := map[string]struct {
		setup       func(t *testing.T) string // returns the test directory
		expectError bool
	}{
		"success_case": {
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				require.NoError(t, exec.Command("git", "init", tmpDir).Run())
				cmd := exec.Command("git", "-C", tmpDir, "remote", "add", "origin", "https://gitlab.cee.redhat.com/app-sre/dynatrace-config.git")
				require.NoError(t, cmd.Run())
				return tmpDir
			},
			expectError: false,
		},
		"git_remote_fails": {
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			expectError: true,
		},
		"invalid_remote_url": {
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				require.NoError(t, exec.Command("git", "init", tmpDir).Run())
				cmd := exec.Command("git", "-C", tmpDir, "remote", "add", "origin", "https://github.com/example/repo.git")
				require.NoError(t, cmd.Run())
				return tmpDir
			},
			expectError: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := tt.setup(t)
			err := checkDynatraceConfigCheckout(dir)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
