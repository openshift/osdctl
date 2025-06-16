package dynatrace

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockExec is a mock implementation of the iexec.IExec interface
type mockExec struct {
	output string
	err    error
}

func (m *mockExec) Run(dir string, name string, args ...string) error {
	return nil
}

func (m *mockExec) Output(dir, cmd string, args ...string) (string, error) {
	return m.output, m.err
}

func (m *mockExec) CombinedOutput(dir, cmd string, args ...string) (string, error) {
	return "", nil
}

func resetBaseDir() {
	// reset package-level variables to allow test re-execution
	baseDirOnce = *new(sync.Once)
	BaseDir = ""
	baseDirErr = nil
}

func TestGetBaseDir_Success(t *testing.T) {
	resetBaseDir()

	mock := &mockExec{
		output: "/path/to/git/repo\n",
		err:    nil,
	}

	dir, err := getBaseDir(mock)

	assert.NoError(t, err)
	assert.Equal(t, "/path/to/git/repo", dir)
	assert.Equal(t, "/path/to/git/repo", BaseDir)
}

func TestGetBaseDir_Error(t *testing.T) {
	resetBaseDir()

	mock := &mockExec{
		output: "",
		err:    errors.New("git error"),
	}

	dir, err := getBaseDir(mock)

	assert.Error(t, err)
	assert.Equal(t, "", dir)
	assert.Equal(t, "git error", err.Error())
}
