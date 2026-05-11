package promote

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServicesRegistry struct", func() {
	var data *TestData
	var servicesRegistry *ServicesRegistry

	BeforeEach(func() {
		data = CreateTestData(func(data *TestData) map[string]string {
			properties := InitProperties(data.TestRepoPath, data.TestRepoHashes[0])

			return map[string]string{
				"data/services/gen-app/cicd/saas/service-1.yaml":        GetFileContent(ServiceFileContentTemplate, "service-1", properties),
				"data/services/gen-app/cicd/saas/service-2.yaml":        GetFileContent(ServiceFileContentTemplate, "service-2", properties),
				"data/services/other-app/cicd/saas/service-3.yaml":      GetFileContent(ServiceFileContentTemplate, "service-3", properties),
				"data/services/gen-app/cicd/saas/service-4/deploy.yaml": GetFileContent(ServiceFileContentTemplate, "service-4", properties),

				"data/services/gen-app/app.yml": GetFileContent(AppFileContentTemplate, "gen-app", properties),
			}
		})
	})

	AfterEach(func() {
		CleanupAllTestDataResources()
	})

	When("the registry is looking at service-1 and service-2 directory only", func() {
		BeforeEach(func() {
			servicesRegistry = CreateDefaultServiceRegistry(data)
		})

		It("finds service-1, service-2 and service-4", func() {
			serviceIds := servicesRegistry.GetServicesIds()
			Expect(serviceIds).To(Equal([]string{"service-1", "service-2", "service-4"}))
		})

		It("returns a valid service object for service-1 and service-2", func() {
			for _, serviceId := range []string{"service-1", "service-2"} {
				service, err := servicesRegistry.GetService(serviceId)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(service).ToNot(BeNil())
				Expect(service.GetFilePath()).To(Equal(filepath.Join(data.AppInterfacePath, "data/services/gen-app/cicd/saas/"+serviceId+".yaml")))
			}
		})

		It("fails to return a valid service object for service-3 even if it was listed", func() {
			for _, serviceId := range []string{"service-3", "service-4"} {
				service, err := servicesRegistry.GetService(serviceId)
				Expect(err).Should(HaveOccurred())
				Expect(service).To(BeNil())
			}
		})
	})

	When("the registry is looking at other directories and in sub-directories", func() {
		BeforeEach(func() {
			servicesRegistry = CreateServiceRegistry(data,
				func(filePath string) string {
					subFilePath := filepath.Join(filePath, "deploy.yaml")
					if fileInfo, err := os.Stat(subFilePath); err == nil && fileInfo.Mode().IsRegular() {
						return subFilePath
					}

					return filePath
				},
				"data/services/gen-app/cicd/saas",
				"data/services/other-app/cicd/saas")
		})

		It("succeeds to find all services", func() {
			for _, serviceId := range []string{"service-1", "service-2", "service-3", "service-4"} {
				service, err := servicesRegistry.GetService(serviceId)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(service).ToNot(BeNil())
			}

			serviceIds := servicesRegistry.GetServicesIds()
			Expect(serviceIds).To(Equal([]string{"service-1", "service-2", "service-3", "service-4"}))
		})
	})
})
