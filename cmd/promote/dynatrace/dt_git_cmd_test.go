package dynatrace

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBaseDir(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) string
		expectError   bool
		expectBaseDir func(t *testing.T, dir string, baseDir string)
	}{
		{
			name: "valid git directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				assert.NoError(t, exec.Command("git", "-C", tmpDir, "init").Run())
				assert.NoError(t, os.Chdir(tmpDir))
				return tmpDir
			},
			expectError: false,
			expectBaseDir: func(t *testing.T, dir string, baseDir string) {
				resolvedTmpDir, err := filepath.EvalSymlinks(dir)
				assert.NoError(t, err)
				resolvedBaseDir, err := filepath.EvalSymlinks(baseDir)
				assert.NoError(t, err)
				assert.Equal(t, resolvedTmpDir, resolvedBaseDir)
			},
		},
		{
			name: "not a git directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				assert.NoError(t, os.Chdir(tmpDir))
				return tmpDir
			},
			expectError: true,
			expectBaseDir: func(t *testing.T, dir string, baseDir string) {
				assert.Equal(t, "", baseDir)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			BaseDir = ""
			baseDirErr = nil
			baseDirOnce = sync.Once{}

			oldDir, _ := os.Getwd()
			defer os.Chdir(oldDir)

			dir := tt.setup(t)
			baseDir, err := getBaseDir()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			tt.expectBaseDir(t, dir, baseDir)
		})
	}
}

func TestCheckBehindMaster(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		expectError bool
	}{
		{
			name: "on master and up to date",
			setup: func(t *testing.T) string {
				upstream := t.TempDir()
				assert.NoError(t, exec.Command("git", "-C", upstream, "init", "--bare").Run())

				local := t.TempDir()
				assert.NoError(t, exec.Command("git", "clone", upstream, local).Run())
				assert.NoError(t, exec.Command("git", "-C", local, "checkout", "-b", "master").Run())

				testFile := filepath.Join(local, "README.md")
				assert.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))
				assert.NoError(t, exec.Command("git", "-C", local, "add", ".").Run())
				assert.NoError(t, exec.Command("git", "-C", local, "commit", "-m", "init commit").Run())
				assert.NoError(t, exec.Command("git", "-C", local, "push", "-u", "origin", "master").Run())
				assert.NoError(t, exec.Command("git", "-C", local, "remote", "add", "upstream", upstream).Run())

				return local
			},
			expectError: false,
		},
		{
			name: "not on master branch",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				exec.Command("git", "-C", dir, "init").Run()
				exec.Command("git", "-C", dir, "checkout", "-b", "dev").Run()
				return dir
			},
			expectError: true,
		},
		{
			name: "no upstream configured",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				exec.Command("git", "-C", dir, "init").Run()
				exec.Command("git", "-C", dir, "checkout", "-b", "master").Run()
				return dir
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)
			err := checkBehindMaster(dir)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
