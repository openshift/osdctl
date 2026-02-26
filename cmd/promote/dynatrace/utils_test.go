package dynatrace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetModulesNames(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (baseDir, subDir string)
		expected    []string
		expectError bool
	}{
		{
			name: "returns_module_names_successfully",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				modulePath := filepath.Join(tmpDir, "modules")
				assert.NoError(t, os.MkdirAll(filepath.Join(modulePath, "moduleA"), 0755))
				assert.NoError(t, os.MkdirAll(filepath.Join(modulePath, "moduleB"), 0755))
				return tmpDir, "modules"
			},
			expected:    []string{"moduleA", "moduleB"},
			expectError: false,
		},
		{
			name: "returns_error_if_dir_does_not_exist",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				return tmpDir, "nonexistent"
			},
			expected:    nil,
			expectError: true,
		},
		{
			name: "returns_empty_if_no_modules",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				modulePath := filepath.Join(tmpDir, "modules")
				assert.NoError(t, os.MkdirAll(modulePath, 0755))
				return tmpDir, "modules"
			},
			expected:    []string{},
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ModulesSlice = nil
			ModulesFilesMap = make(map[string]string)
			baseDir, subDir := tc.setup(t)
			result, err := GetModulesNames(baseDir, subDir)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tc.expected, result)
			}
		})
	}
}
