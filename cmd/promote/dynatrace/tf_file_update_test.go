package dynatrace_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift/osdctl/cmd/promote/dynatrace"

	"os"

	"github.com/hashicorp/hcl/v2/hclwrite"
)

var _ = Describe("Dynatrace", func() {
	var testFilePath string
	var tempDir string

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "dynatrace-test-")
		Expect(err).NotTo(HaveOccurred())
		testFilePath = filepath.Join(tempDir, "test.hcl")
		content := `module "example" { source = "old_value" }`
		_ = os.WriteFile(testFilePath, []byte(content), 0644)
	})

	AfterEach(func() {
		_ = os.Remove(testFilePath)
		_ = os.RemoveAll(tempDir)
	})

	Describe("Open", func() {
		BeforeEach(func() {
			content := `module "example" { source = "old_value" }`
			_ = os.WriteFile(testFilePath, []byte(content), 0644)
		})

		It("should open an existing HCL file successfully", func() {
			_, err := dynatrace.Open(testFilePath)
			Expect(err).To(BeNil())
			// Expect(file).To(BeTrue())
		})

		It("should return an error for a non-existing file", func() {
			_, err := dynatrace.Open("non_existent.hcl")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UpdateDefaultValue", func() {
		var file *hclwrite.File

		BeforeEach(func() {
			var err error
			file, err = dynatrace.Open(testFilePath)
			Expect(err).To(BeNil())
		})

		It("should update the source attribute of the matching module", func() {
			updated := dynatrace.UpdateDefaultValue(file, "example", "new_value")
			Expect(updated).To(BeTrue())

			attr := file.Body().Blocks()[0].Body().GetAttribute("source")
			Expect(attr).NotTo(BeNil())
			Expect(attr.Expr().BuildTokens(nil).Bytes()).To(ContainSubstring("new_value"))
		})

		It("should return false if the module is not found", func() {
			updated := dynatrace.UpdateDefaultValue(file, "nonexistent", "new_value")
			Expect(updated).To(BeFalse())
		})
	})

	Describe("Save", func() {
		Describe("Save", func() {
			It("should save the file successfully", func() {
				tempDir, err := os.MkdirTemp("", "dynatrace-test-")
				Expect(err).NotTo(HaveOccurred())
				savePath := filepath.Join(tempDir, "output.hcl")
				file := hclwrite.NewEmptyFile()
				err = dynatrace.Save(savePath, file)
				Expect(err).To(BeNil())
				_, err = os.Stat(savePath)
				Expect(err).NotTo(HaveOccurred())
				_ = os.Remove(savePath)
				_ = os.RemoveAll(tempDir)
			})
		})

		It("should return an error if the file cannot be written", func() {
			file := hclwrite.NewEmptyFile()
			_ = os.Mkdir("output.hcl", 0755) // Create a directory instead of a file
			err := dynatrace.Save("output.hcl", file)
			Expect(err).To(HaveOccurred())
			_ = os.Remove("output.hcl")
		})
	})
})
