package dynatrace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckoutAndCompareGitHash(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (repoPath, latestHash, currentHash string)
		expectErr   bool
		expectMatch func(t *testing.T, hash, log string)
	}{
		{
			name: "success_case",
			setup: func(t *testing.T) (string, string, string) {
				tempDir := t.TempDir()
				assert.NoError(t, exec.Command("git", "init").Run())
				cmd := exec.Command("git", "init")
				cmd.Dir = tempDir
				assert.NoError(t, cmd.Run())
				readme := filepath.Join(tempDir, "README.md")
				assert.NoError(t, os.WriteFile(readme, []byte("Initial content"), 0644))
				assert.NoError(t, exec.Command("git", "-C", tempDir, "add", ".").Run())
				assert.NoError(t, exec.Command("git", "-C", tempDir, "commit", "-m", "initial commit").Run())
				out, err := exec.Command("git", "-C", tempDir, "rev-parse", "HEAD").Output()
				assert.NoError(t, err)
				initialHash := strings.TrimSpace(string(out))
				newFile := filepath.Join(tempDir, "newfile.txt")
				assert.NoError(t, os.WriteFile(newFile, []byte("new content"), 0644))
				assert.NoError(t, exec.Command("git", "-C", tempDir, "add", ".").Run())
				assert.NoError(t, exec.Command("git", "-C", tempDir, "commit", "-m", "new commit").Run())
				out, err = exec.Command("git", "-C", tempDir, "rev-parse", "HEAD").Output()
				assert.NoError(t, err)
				latestHash := strings.TrimSpace(string(out))
				return tempDir, latestHash, initialHash
			},
			expectErr: false,
			expectMatch: func(t *testing.T, hash, log string) {
				assert.NotEmpty(t, log)
				assert.Contains(t, log, "new commit")
			},
		},
		{
			name: "same_git_hash_returns_error",
			setup: func(t *testing.T) (string, string, string) {
				tempDir := t.TempDir()
				cmd := exec.Command("git", "init")
				cmd.Dir = tempDir
				assert.NoError(t, cmd.Run())
				filePath := filepath.Join(tempDir, "README.md")
				assert.NoError(t, os.WriteFile(filePath, []byte("Initial content"), 0644))
				assert.NoError(t, exec.Command("git", "-C", tempDir, "add", ".").Run())
				assert.NoError(t, exec.Command("git", "-C", tempDir, "commit", "-m", "initial commit").Run())
				out, err := exec.Command("git", "-C", tempDir, "rev-parse", "HEAD").Output()
				assert.NoError(t, err)
				hash := strings.TrimSpace(string(out))
				return tempDir, hash, hash
			},
			expectErr: true,
			expectMatch: func(t *testing.T, hash, log string) {
				assert.Empty(t, log)
			},
		},
		{
			name: "invalid_repo_returns_error",
			setup: func(t *testing.T) (string, string, string) {
				return filepath.Join(t.TempDir(), "nonexistent"), "", ""
			},
			expectErr: true,
			expectMatch: func(t *testing.T, hash, log string) {
				assert.Empty(t, log)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath, gitHash, currentHash := tt.setup(t)
			hash, log, err := CheckoutAndCompareGitHash(repoPath, gitHash, currentHash, ".")
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			tt.expectMatch(t, hash, log)
		})
	}
}
