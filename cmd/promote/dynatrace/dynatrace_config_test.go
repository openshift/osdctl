package dynatrace

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testMockExec struct {
	mock.Mock
}

func (m *testMockExec) Run(dir string, name string, args ...string) error {
	argsList := m.Called(dir, name, args)
	return argsList.Error(0)
}

func (m *testMockExec) Output(dir, cmd string, args ...string) (string, error) {
	argsList := m.Called(dir, cmd, args)
	return argsList.String(0), argsList.Error(1)
}

func (m *testMockExec) CombinedOutput(dir, cmd string, args ...string) (string, error) {
	argsList := m.Called(dir, cmd, args)
	return argsList.String(0), argsList.Error(1)
}

func TestCheckDynatraceConfigCheckout(t *testing.T) {
	tests := []struct {
		name        string
		mockOutput  string
		mockErr     error
		expectError bool
		errorMsg    string
	}{
		{
			name:        "success_valid_dynatrace-config_url",
			mockOutput:  "origin git@gitlab.cee.redhat.com:dynatrace-config.git (fetch)",
			mockErr:     nil,
			expectError: false,
		},
		{
			name:        "error_invalid_remote",
			mockOutput:  "origin git@github.com:someuser/other-repo.git (fetch)",
			mockErr:     nil,
			expectError: true,
			errorMsg:    "not running in checkout of dynatrace-config",
		},
		{
			name:        "error_command_execution_failure",
			mockOutput:  "",
			mockErr:     errors.New("simulated command failure"),
			expectError: true,
			errorMsg:    "error executing 'git remote -v'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockExec := new(testMockExec)
			mockExec.On("CombinedOutput", "/fake/dir", "git", []string{"remote", "-v"}).
				Return(tc.mockOutput, tc.mockErr)

			cfg := DynatraceConfig{
				GitDirectory: "/fake/dir",
				GitExecutor:  mockExec,
			}

			err := cfg.checkDynatraceConfigCheckout()
			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCommitFiles(t *testing.T) {
	tests := map[string]struct {
		setup       func() DynatraceConfig
		commitMsg   string
		expectError bool
	}{
		"successfully_commits_file": {
			setup: func() DynatraceConfig {
				mockExec := new(testMockExec)
				mockExec.On("Run", "some-dir", "git", []string{"add", "."}).Return(nil)
				mockExec.On("Run", "some-dir", "git", []string{"commit", "-m", "initial commit"}).Return(nil)
				return DynatraceConfig{GitDirectory: "some-dir", GitExecutor: mockExec}
			},
			commitMsg:   "initial commit",
			expectError: false,
		},
		"fails_when_git_add_fails": {
			setup: func() DynatraceConfig {
				mockExec := new(testMockExec)
				mockExec.On("Run", "invalid-dir", "git", []string{"add", "."}).Return(fmt.Errorf("git not initialized"))
				return DynatraceConfig{GitDirectory: "invalid-dir", GitExecutor: mockExec}
			},
			commitMsg:   "should fail",
			expectError: true,
		},
		"fails_when_git_commit_fails": {
			setup: func() DynatraceConfig {
				mockExec := new(testMockExec)
				mockExec.On("Run", "some-dir", "git", []string{"add", "."}).Return(nil)
				mockExec.On("Run", "some-dir", "git", []string{"commit", "-m", "initial commit"}).Return(fmt.Errorf("git commit failed"))
				return DynatraceConfig{GitDirectory: "some-dir", GitExecutor: mockExec}
			},
			commitMsg:   "initial commit",
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := tc.setup()
			err := cfg.commitFiles(tc.commitMsg)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
