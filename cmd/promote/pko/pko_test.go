package pko

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/openshift/osdctl/cmd/promote/git"
	"github.com/openshift/osdctl/cmd/promote/saas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromotePackage_Success(t *testing.T) {
	// Setup temp Git directory
	tmpDir := t.TempDir()

	// Init git repo
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = tmpDir
		require.NoError(t, cmd.Run())
	}

	// Create SAAS file
	saasDir := filepath.Join(tmpDir, "data/services/osd-operators/cicd/saas")
	require.NoError(t, os.MkdirAll(saasDir, 0755))
	saasFilePath := filepath.Join(saasDir, "saas-test.yaml")

	content := `
name: test
resourceTemplates:
  - name: test-template
    targets:
      - namespace:
          $ref: some/path/prod-hive
        parameters:
          PACKAGE_TAG: old123
`
	require.NoError(t, os.WriteFile(saasFilePath, []byte(content), 0644))
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())
	appInterface := git.AppInterface{
		GitDirectory: tmpDir,
	}
	saas.ServicesSlice = nil
	saas.ServicesFilesMap = map[string]string{}
	err := PromotePackage(appInterface, "test", "new456", false)
	require.NoError(t, err)

	updatedContent, err := os.ReadFile(filepath.Join(saasDir, "saas-test.yaml"))
	require.NoError(t, err)

	assert.Contains(t, string(updatedContent), "new456")
}

func TestPromotePackage_ServiceNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Init git repo
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = tmpDir
		require.NoError(t, cmd.Run())
	}
	saasDir := filepath.Join(tmpDir, "data/services/osd-operators/cicd/saas")
	require.NoError(t, os.MkdirAll(saasDir, 0755))
	saasFilePath := filepath.Join(saasDir, "saas-actual.yaml")

	content := `
name: actual
resourceTemplates:
  - name: test-template
    targets:
      - namespace:
          $ref: some/path/prod-hive
        parameters:
          PACKAGE_TAG: old123
`
	require.NoError(t, os.WriteFile(saasFilePath, []byte(content), 0644))

	cmd := exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	appInterface := git.AppInterface{GitDirectory: tmpDir}
	saas.ServicesSlice = nil
	saas.ServicesFilesMap = map[string]string{}
	err := PromotePackage(appInterface, "non-existent", "new456", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPromotePackage_InvalidYamlSyntax(t *testing.T) {
	tmpDir := t.TempDir()

	// Init git repo
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = tmpDir
		require.NoError(t, cmd.Run())
	}
	saasDir := filepath.Join(tmpDir, "data/services/osd-operators/cicd/saas")
	require.NoError(t, os.MkdirAll(saasDir, 0755))
	saasFilePath := filepath.Join(saasDir, "saas-broken.yaml")

	badYaml := `
name: broken
resourceTemplates:
  - name: broken-template
    targets:
      - namespace:
          $ref: some/path/prod-hive
        parameters:
          PACKAGE_TAG: old123
      - invalid_yaml_entry_here: [unclosed
`
	require.NoError(t, os.WriteFile(saasFilePath, []byte(badYaml), 0644))

	// Git add/commit
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "commit", "-m", "broken yaml commit")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	appInterface := git.AppInterface{GitDirectory: tmpDir}
	saas.ServicesSlice = nil
	saas.ServicesFilesMap = map[string]string{}

	err := PromotePackage(appInterface, "broken", "new456", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "yaml")
}
