package git

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/osdctl/cmd/promote/iexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockExec is a mock for the executor used in the git package
type MockExecService struct {
	mock.Mock
	iexec.IExec
}

func (m *MockExecService) Run(dir string, command string, args ...string) error {
	argsList := m.Called(dir, command, args)
	return argsList.Error(0)
}

func (m *MockExecService) Output(dir string, command string, args ...string) (string, error) {
	argsList := m.Called(dir, command, args)
	return argsList.String(0), argsList.Error(1)
}

func (m *MockExecService) CombinedOutput(dir string, command string, args ...string) (string, error) {
	argsList := m.Called(dir, command, args)
	return argsList.String(0), argsList.Error(1)
}

func TestCheckoutAndCompareGitHash_ChdirAndGitCloneError(t *testing.T) {
	mockExec := new(MockExecService)

	mockExec.On("Run", mock.Anything, "git", mock.MatchedBy(func(args []string) bool {
		return len(args) == 3 && args[0] == "clone" && args[1] == "https://github.com/some/repo.git"
	})).Return(fmt.Errorf("simulated clone failure")).Once()

	_, _, err := CheckoutAndCompareGitHash(
		mockExec,
		"https://github.com/some/repo.git",
		"abcdef1234567890",
		"abcdef1234567890",
		"service/path",
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to clone git repository")

	mockExec.AssertExpectations(t)
}

func TestCheckoutAndCompareGitHash_ChdirSourceDirError(t *testing.T) {
	mockExec := new(MockExecService)

	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	sourceDir := filepath.Join(tempDir, "source-dir")
	err = os.Mkdir(sourceDir, 0o755)
	require.NoError(t, err)

	mockExec.On("Run", mock.Anything, "git", mock.MatchedBy(func(args []string) bool {
		return len(args) == 3 && args[0] == "clone" && args[1] == "https://github.com/some/repo.git"
	})).Return(nil).Once()

	_, _, err = CheckoutAndCompareGitHash(mockExec, "https://github.com/some/repo.git", "", "", "service/path")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to change directory to source-dir")
}
