package saas

import (
	"fmt"
	"strings"
	"testing"

	"github.com/openshift/osdctl/pkg/promote"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var serviceFileContentCanaryTemplate = strings.Replace(promote.ServiceFileContentTemplate,
	"name: hivep01",
	"name: hivep01"+defaultProdTargetNameSuffix,
	1)

func TestSetup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Setup Suite")
}

var _ = Describe("ServicesRegistry struct", func() {
	var data *promote.TestData
	var servicesRegistry *promote.ServicesRegistry

	BeforeEach(func() {
		data = promote.CreateTestData(func(data *promote.TestData) map[string]string {
			properties := promote.InitProperties(data.TestRepoPath, data.TestRepoHashes[0])
			return map[string]string{
				"data/services/gen-app/cicd/saas/saas-service-1.yaml":          promote.GetFileContent(promote.ServiceFileContentTemplate, "service-1", properties),
				"data/services/gen-app/cicd/saas/service-2.yaml":               promote.GetFileContent(promote.ServiceFileContentTemplate, "service-2", properties),
				"data/services/other-app/cicd/saas/saas-service-3/deploy.yaml": promote.GetFileContent(promote.ServiceFileContentTemplate, "service-3", properties),
				"data/services/gen-app/cicd/saas/service-4/deploy.yaml":        promote.GetFileContent(promote.ServiceFileContentTemplate, "service-4", properties),

				"data/services/gen-app/app.yml": promote.GetFileContent(promote.AppFileContentTemplate, "gen-app", properties),
			}
		})
		servicesRegistry = promote.CreateServiceRegistry(data,
			validateSaasServiceFilePath,
			"data/services/gen-app/cicd/saas",
			"data/services/other-app/cicd/saas")
	})

	AfterEach(func() {
		promote.CleanupAllTestDataResources()
	})

	When("querying the registry", func() {
		It("finds saas-service-1 and saas-service-3 services only", func() {
			serviceIds := servicesRegistry.GetServicesIds()
			Expect(serviceIds).To(Equal([]string{"saas-service-1", "saas-service-3"}))
		})

		It("returns a valid service object for both services", func() {
			for _, serviceId := range []string{"saas-service-1", "saas-service-3"} {
				service, err := servicesRegistry.GetService(serviceId)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(service).ToNot(BeNil())
			}
		})
	})
})

var _ = Describe("Service struct", func() {
	var data *promote.TestData
	var service *promote.Service

	BeforeEach(func() {
		data = promote.CreateTestData(func(data *promote.TestData) map[string]string {
			properties := promote.InitProperties(data.TestRepoPath, data.TestRepoHashes[0])

			return map[string]string{
				"data/services/gen-app/cicd/saas/service-1.yaml": promote.GetFileContent(serviceFileContentCanaryTemplate, "service-1", properties),
				"data/services/gen-app/app.yml":                  promote.GetFileContent(promote.AppFileContentTemplate, "gen-app", properties),
			}
		})
	})

	JustBeforeEach(func() {
		var err error

		servicesRegistry := promote.CreateDefaultServiceRegistry(data)
		service, err = servicesRegistry.GetService("service-1")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(service).ToNot(BeNil())
	})

	AfterEach(func() {
		promote.CleanupAllTestDataResources()
	})

	Context("Promote method", func() {
		When("namespaceRef is set to 'hivep'", func() {
			It("promotes all targets in all resource templates", func() { // because all namespaces have their ref contain that string
				err := service.Promote(&promoteCallbacks{
					DefaultPromoteCallbacks: promote.DefaultPromoteCallbacks{Service: service},
					namespaceRef:            promote.DefaultProdNamespaceRef,
					isHotfix:                false,
				}, data.TestRepoHashes[5])
				Expect(err).ShouldNot(HaveOccurred())

				Expect(data.GetAppInterfaceCommitsCount()).To(Equal(3))

				data.CheckAppInterfaceCommitMessage(0, "&var-saasfilename=saas-default-component-e2e-test&")
			})

			When("there is a E2E service file in a sub-directory of the service directory", func() {
				BeforeEach(func() {
					data.WriteAppInterfaceFile("data/services/gen-app/cicd/saas/service-1/osde2e-focus-test.yaml", "name: default-e2e-service\n")
					data.CommitAppInterfaceChanges("Defining an E2E service file")
				})

				It("still promotes all targets in all resource templates but the links in the commit message change a bit", func() {
					err := service.Promote(&promoteCallbacks{
						DefaultPromoteCallbacks: promote.DefaultPromoteCallbacks{Service: service},
						namespaceRef:            promote.DefaultProdNamespaceRef,
						isHotfix:                false,
					}, data.TestRepoHashes[5])
					Expect(err).ShouldNot(HaveOccurred())

					Expect(data.GetAppInterfaceCommitsCount()).To(Equal(4))

					data.CheckAppInterfaceCommitMessage(0, "&var-saasfilename=default-e2e-service&")
				})
			})

			AfterEach(func() {
				data.CheckAppInterfaceService1Content(serviceFileContentCanaryTemplate, promote.InitProperties(data.TestRepoPath, data.TestRepoHashes[5]))

				data.CheckAppInterfaceIsClean()
				data.CheckAppInterfaceBranchName(fmt.Sprintf("promote-service-1-%s", data.TestRepoHashes[5]))

				data.CheckAppInterfaceCommitMessage(0, data.GetTestRepoFormattedLog(5, 2))
				data.CheckAppInterfaceCommitMessage(0, "?var-namespace=default-component-pipelines&")

				data.CheckAppInterfaceCommitStats(0, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 2, 2)

				data.CheckAppInterfaceCommitMessage(1, data.GetTestRepoFormattedLog(4, 1))
				data.CheckAppInterfaceCommitStats(1, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 2, 2)
			})
		})

		When("namespaceRef is set to 'hivep02'", func() {
			It("only promotes those hivep02 targets", func() {
				err := service.Promote(&promoteCallbacks{
					DefaultPromoteCallbacks: promote.DefaultPromoteCallbacks{Service: service},
					namespaceRef:            "hivep02",
					isHotfix:                false,
				}, data.TestRepoHashes[8])
				Expect(err).ShouldNot(HaveOccurred())

				expectedProperties := promote.InitProperties(data.TestRepoPath, data.TestRepoHashes[0])
				expectedProperties["gitHashProd1Target2"] = data.TestRepoHashes[8]
				expectedProperties["gitHashProd2Target2"] = data.TestRepoHashes[8]
				data.CheckAppInterfaceService1Content(serviceFileContentCanaryTemplate, expectedProperties)

				data.CheckAppInterfaceIsClean()
				data.CheckAppInterfaceBranchName(fmt.Sprintf("promote-service-1-%s", data.TestRepoHashes[8]))

				Expect(data.GetAppInterfaceCommitsCount()).To(Equal(3))

				data.CheckAppInterfaceCommitMessage(0, data.GetTestRepoFormattedLog(5, 2))
				data.CheckAppInterfaceCommitStats(0, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 1, 1)

				data.CheckAppInterfaceCommitMessage(1, data.GetTestRepoFormattedLog(4, 1))
				data.CheckAppInterfaceCommitStats(1, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 1, 1)
			})
		})

		When("namespaceRef is empty", func() {
			It("only promotes the canary target", func() {
				err := service.Promote(&promoteCallbacks{
					DefaultPromoteCallbacks: promote.DefaultPromoteCallbacks{Service: service},
					namespaceRef:            "", // empty namespaceRef means only considering the canary target
					isHotfix:                false,
				}, data.TestRepoHashes[9])
				Expect(err).ShouldNot(HaveOccurred())

				expectedProperties := promote.InitProperties(data.TestRepoPath, data.TestRepoHashes[0])
				expectedProperties["gitHashProd1Target1"] = data.TestRepoHashes[9]
				data.CheckAppInterfaceService1Content(serviceFileContentCanaryTemplate, expectedProperties)

				data.CheckAppInterfaceIsClean()
				data.CheckAppInterfaceBranchName(fmt.Sprintf("promote-service-1-%s", data.TestRepoHashes[9]))

				Expect(data.GetAppInterfaceCommitsCount()).To(Equal(2))

				data.CheckAppInterfaceCommitMessage(0, data.GetTestRepoFormattedLog(7, 4, 1))
				data.CheckAppInterfaceCommitStats(0, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 1, 1)
			})
		})

		When("there is a hotfix", func() {
			It("promotes all targets in all resource templates & update the application file", func() {
				err := service.Promote(&promoteCallbacks{
					DefaultPromoteCallbacks: promote.DefaultPromoteCallbacks{Service: service},
					namespaceRef:            "", // empty namespaceRef normally means that only the canary target is considered, but in case of hotfix, this default to "hivep"
					isHotfix:                true,
				}, data.TestRepoHashes[9])
				Expect(err).ShouldNot(HaveOccurred())

				data.CheckAppInterfaceService1Content(serviceFileContentCanaryTemplate, promote.InitProperties(data.TestRepoPath, data.TestRepoHashes[9]))

				expectedAppProperties := promote.InitProperties(data.TestRepoPath, "")
				expectedAppProperties["hotfixVersion"] = data.TestRepoHashes[9]
				data.CheckAppInterfaceFileContent("data/services/gen-app/app.yml", promote.AppFileContentTemplateWithHotfixVersion, "gen-app", expectedAppProperties)

				data.CheckAppInterfaceIsClean()
				data.CheckAppInterfaceBranchName(fmt.Sprintf("promote-service-1-%s", data.TestRepoHashes[9]))

				Expect(data.GetAppInterfaceCommitsCount()).To(Equal(3))

				data.CheckAppInterfaceCommitMessage(0, data.GetTestRepoFormattedLog(8, 5, 2))
				data.CheckAppInterfaceCommitStats(0, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 2, 2)

				data.CheckAppInterfaceCommitMessage(1, data.GetTestRepoFormattedLog(7, 4, 1))
				data.CheckAppInterfaceCommitStats(1, 2, "data/services/gen-app/cicd/saas/service-1.yaml", 2, 2)
				data.CheckAppInterfaceCommitStats(1, 2, "data/services/gen-app/app.yml", 2, 0)
			})
		})
	})
})
