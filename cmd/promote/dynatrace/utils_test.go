package dynatrace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

var _ = ginkgo.Describe("Dynatrace Utilities", func() {

	var mockAppInterface AppInterface

	ginkgo.BeforeEach(func() {
		// Here you can initialize the mock AppInterface (if needed)
		mockAppInterface = AppInterface{
			GitDirectory: "/mock/base/dir",
		}
	})

	ginkgo.Describe("GetServiceNames", func() {
		var (
			mockDirs = []string{"dir1", "dir2"}
		)

		ginkgo.It("should return a list of service names", func() {
			services, err := GetServiceNames(mockAppInterface, mockDirs...)
			gomega.Expect(err).To(gomega.BeNil())
			gomega.Expect(services).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("ValidateServiceName", func() {
		var services = []string{"service1", "dynatrace-service2"}

		ginkgo.It("should find exact match for service name", func() {
			service, err := ValidateServiceName(services, "service1")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(service).To(gomega.Equal("service1"))
		})

		ginkgo.It("should find dynatrace-prefixed service name", func() {
			service, err := ValidateServiceName(services, "service2")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(service).To(gomega.Equal("dynatrace-service2"))
		})

		ginkgo.It("should return an error for non-matching service names", func() {
			_, err := ValidateServiceName(services, "service3")
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).To(gomega.Equal("service service3 not found"))
		})
	})

	ginkgo.Describe("GetSaasDir", func() {
		ginkgo.It("should return the correct path for a service", func() {
			ServicesFilesMap = map[string]string{
				"service1": "/mock/path/service1.yaml",
			}

			path, err := GetSaasDir("service1")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(path).To(gomega.Equal("/mock/path/service1.yaml"))
		})

		ginkgo.It("should return an error if service does not exist", func() {
			_, err := GetSaasDir("nonexistentService")
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).To(gomega.Equal("saas directory for service nonexistentService not found"))
		})

		ginkgo.It("should return an error if file path does not contain .yaml", func() {
			ServicesFilesMap = map[string]string{
				"service1": "/mock/path/service1.txt", // Not a .yaml file
			}

			_, err := GetSaasDir("service1")
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).To(gomega.Equal("saas directory for service service1 not found"))
		})
	})

})

func TestGeModulesNames(t *testing.T) {
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
			result, err := GeModulesNames(baseDir, subDir)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tc.expected, result)
			}
		})
	}
}
