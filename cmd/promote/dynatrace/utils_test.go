package dynatrace

import (
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
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
