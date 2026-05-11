package promote

import (
	"fmt"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

type customPromoteCallbacks struct {
	DefaultPromoteCallbacks

	targetName string
}

func (c *customPromoteCallbacks) FilterTargets(targetNodes []*kyaml.RNode) ([]*kyaml.RNode, error) {
	var filteredTargetNodes []*kyaml.RNode

	for _, targetNode := range targetNodes {
		targetName, err := targetNode.GetString("name")
		if err != nil {
			fmt.Printf("Path 'resourceTemplates[].targets[].name' is not always defined as a string in '%s': %v\n", c.Service.GetFilePath(), err)
			continue
		}
		if targetName == c.targetName {
			filteredTargetNodes = append(filteredTargetNodes, targetNode)
		}
	}

	return filteredTargetNodes, nil
}

var _ = Describe("Application struct", func() {
	var data *TestData

	BeforeEach(func() {
		data = CreateDefaultTestData()
	})

	AfterEach(func() {
		CleanupAllTestDataResources()
	})

	Context("Using getters", func() {
		It("returns the expected values", func() {
			application, err := readApplicationFromFile(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(application).ToNot(BeNil())

			Expect(application.GetFilePath()).To(Equal(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml")))
			Expect(application.GetName()).To(Equal("gen-app"))

			component, err := application.GetComponent(data.TestRepoPath)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(component.GetName()).To(Equal("default-component"))
		})
	})

	Context("Using GetComponentNames", func() {
		It("returns all component names in order", func() {
			application, err := readApplicationFromFile(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(application).ToNot(BeNil())

			names, err := application.GetComponentNames()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(names).To(Equal([]string{"dummy-component", "default-component"}))
		})
	})

	Context("Using GetAllComponents", func() {
		It("returns all components in order", func() {
			application, err := readApplicationFromFile(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(application).ToNot(BeNil())

			components, err := application.GetAllComponents()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(components).To(HaveLen(2))
			Expect(components[0].GetName()).To(Equal("dummy-component"))
			Expect(components[1].GetName()).To(Equal("default-component"))
		})
	})

	Context("Using SetHotfixVersion and Save", func() {
		var hotfixVersion string

		BeforeEach(func() {
			hotfixVersion = "1.2.3"
		})

		It("succeeds if no hotfix version is yet set in the application file", func() {
		})

		It("succeeds and replace the hotfix version set in the application file", func() {
			properties := InitProperties(data.TestRepoPath, "")
			properties["hotfixVersion"] = "1.0.0"
			appFileContent := GetFileContent(AppFileContentTemplateWithHotfixVersion, "gen-app", properties)
			data.WriteAppInterfaceFile("data/services/gen-app/app.yml", appFileContent)
		})

		It("succeeds and replace the hotfix versions set in the application file", func() {
			properties := InitProperties(data.TestRepoPath, "")
			properties["hotfixVersion1"] = "1.0.0"
			properties["hotfixVersion2"] = "1.0.1"
			appFileContent := GetFileContent(AppFileContentTemplateWithHotfixVersions, "gen-app", properties)
			data.WriteAppInterfaceFile("data/services/gen-app/app.yml", appFileContent)
		})

		AfterEach(func() {
			application, err := readApplicationFromFile(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml"))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(application).ToNot(BeNil())

			component, err := application.GetComponent(data.TestRepoPath)
			Expect(err).ShouldNot(HaveOccurred())
			err = component.SetHotfixVersion(hotfixVersion)
			Expect(err).ShouldNot(HaveOccurred())

			// Making the call
			err = application.Save()
			Expect(err).ShouldNot(HaveOccurred())

			expectedProperties := InitProperties(data.TestRepoPath, "")
			expectedProperties["hotfixVersion"] = hotfixVersion
			expectedAppFileContent := GetFileContent(AppFileContentTemplateWithHotfixVersion, "gen-app", expectedProperties)

			Expect(data.ReadAppInterfaceFile("data/services/gen-app/app.yml")).To(Equal(expectedAppFileContent))
		})
	})
})

var _ = Describe("Application.GetComponentByName", func() {
	var data *TestData

	BeforeEach(func() {
		data = CreateDefaultTestData()
	})

	AfterEach(func() {
		CleanupAllTestDataResources()
	})

	It("returns the component when found by name", func() {
		application, err := readApplicationFromFile(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml"))
		Expect(err).ShouldNot(HaveOccurred())

		component, err := application.GetComponentByName("default-component")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(component).ToNot(BeNil())
		Expect(component.GetName()).To(Equal("default-component"))
	})

	It("returns the dummy-component when found by name", func() {
		application, err := readApplicationFromFile(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml"))
		Expect(err).ShouldNot(HaveOccurred())

		component, err := application.GetComponentByName("dummy-component")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(component).ToNot(BeNil())
		Expect(component.GetName()).To(Equal("dummy-component"))
	})

	It("returns an error when the component name does not exist", func() {
		application, err := readApplicationFromFile(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml"))
		Expect(err).ShouldNot(HaveOccurred())

		component, err := application.GetComponentByName("nonexistent-component")
		Expect(err).Should(HaveOccurred())
		Expect(component).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("nonexistent-component"))
	})
})

var _ = Describe("CodeComponent.AddBlockedVersion", func() {
	var data *TestData

	BeforeEach(func() {
		data = CreateDefaultTestData()
	})

	AfterEach(func() {
		CleanupAllTestDataResources()
	})

	Context("when no blockedVersions field exists yet", func() {
		It("creates the field with a single entry", func() {
			application, err := readApplicationFromFile(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml"))
			Expect(err).ShouldNot(HaveOccurred())

			component, err := application.GetComponentByName("default-component")
			Expect(err).ShouldNot(HaveOccurred())

			err = component.AddBlockedVersion("abc123")
			Expect(err).ShouldNot(HaveOccurred())

			err = application.Save()
			Expect(err).ShouldNot(HaveOccurred())

			expectedProperties := InitProperties(data.TestRepoPath, "")
			expectedProperties["blockedVersion"] = "abc123"
			expectedAppFileContent := GetFileContent(AppFileContentTemplateWithBlockedVersion, "gen-app", expectedProperties)
			Expect(data.ReadAppInterfaceFile("data/services/gen-app/app.yml")).To(Equal(expectedAppFileContent))
		})
	})

	Context("when blockedVersions already has one entry", func() {
		BeforeEach(func() {
			properties := InitProperties(data.TestRepoPath, "")
			properties["blockedVersion"] = "existing123"
			appFileContent := GetFileContent(AppFileContentTemplateWithBlockedVersion, "gen-app", properties)
			data.WriteAppInterfaceFile("data/services/gen-app/app.yml", appFileContent)
		})

		It("appends a second entry", func() {
			application, err := readApplicationFromFile(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml"))
			Expect(err).ShouldNot(HaveOccurred())

			component, err := application.GetComponentByName("default-component")
			Expect(err).ShouldNot(HaveOccurred())

			err = component.AddBlockedVersion("newblock456")
			Expect(err).ShouldNot(HaveOccurred())

			err = application.Save()
			Expect(err).ShouldNot(HaveOccurred())

			expectedProperties := InitProperties(data.TestRepoPath, "")
			expectedProperties["blockedVersion1"] = "existing123"
			expectedProperties["blockedVersion2"] = "newblock456"
			expectedAppFileContent := GetFileContent(AppFileContentTemplateWithBlockedVersions, "gen-app", expectedProperties)
			Expect(data.ReadAppInterfaceFile("data/services/gen-app/app.yml")).To(Equal(expectedAppFileContent))
		})

		It("returns an error when the version already exists", func() {
			application, err := readApplicationFromFile(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml"))
			Expect(err).ShouldNot(HaveOccurred())

			component, err := application.GetComponentByName("default-component")
			Expect(err).ShouldNot(HaveOccurred())

			err = component.AddBlockedVersion("existing123")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already in"))
			Expect(err.Error()).To(ContainSubstring("existing123"))
		})
	})

	Context("when blockedVersions already has two entries", func() {
		BeforeEach(func() {
			properties := InitProperties(data.TestRepoPath, "")
			properties["blockedVersion1"] = "first111"
			properties["blockedVersion2"] = "second222"
			appFileContent := GetFileContent(AppFileContentTemplateWithBlockedVersions, "gen-app", properties)
			data.WriteAppInterfaceFile("data/services/gen-app/app.yml", appFileContent)
		})

		It("appends a third entry", func() {
			application, err := readApplicationFromFile(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml"))
			Expect(err).ShouldNot(HaveOccurred())

			component, err := application.GetComponentByName("default-component")
			Expect(err).ShouldNot(HaveOccurred())

			err = component.AddBlockedVersion("third333")
			Expect(err).ShouldNot(HaveOccurred())

			err = application.Save()
			Expect(err).ShouldNot(HaveOccurred())

			// Read back and verify all three are present
			actualContent := data.ReadAppInterfaceFile("data/services/gen-app/app.yml")
			Expect(actualContent).To(ContainSubstring("first111"))
			Expect(actualContent).To(ContainSubstring("second222"))
			Expect(actualContent).To(ContainSubstring("third333"))
		})

		It("rejects a duplicate of the first entry", func() {
			application, err := readApplicationFromFile(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml"))
			Expect(err).ShouldNot(HaveOccurred())

			component, err := application.GetComponentByName("default-component")
			Expect(err).ShouldNot(HaveOccurred())

			err = component.AddBlockedVersion("second222")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already in"))
		})
	})
})

var _ = Describe("Service struct", func() {
	var data *TestData
	var service *Service

	BeforeEach(func() {
		data = CreateDefaultTestData()
	})

	JustBeforeEach(func() {
		var err error

		servicesRegistry := CreateDefaultServiceRegistry(data)
		service, err = servicesRegistry.GetService("service-1")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(service).ToNot(BeNil())
	})

	AfterEach(func() {
		CleanupAllTestDataResources()
	})

	Context("Using getters", func() {
		It("returns the expected values", func() {
			Expect(service.GetFilePath()).To(Equal(filepath.Join(data.AppInterfacePath, "data/services/gen-app/cicd/saas/service-1.yaml")))
			Expect(service.GetName()).To(Equal("service-1"))

			application := service.GetApplication()
			Expect(application).ToNot(BeNil())
			Expect(application.GetFilePath()).To(Equal(filepath.Join(data.AppInterfacePath, "data/services/gen-app/app.yml")))

			Expect(service.GetRootNode()).ToNot(BeNil())
			Expect(service.GetResourceTemplatesSequenceNode()).ToNot(BeNil())
		})
	})

	Context("Promote method", func() {
		When("using default callbacks to consider all targets", func() {
			When("all targets share the same git hash", func() {
				It("promotes all targets in all resource templates", func() {
					err := service.Promote(&DefaultPromoteCallbacks{service}, data.TestRepoHashes[7])
					Expect(err).ShouldNot(HaveOccurred())

					Expect(data.GetAppInterfaceCommitsCount()).To(Equal(3))

					data.CheckAppInterfaceCommitMessage(0, data.GetTestRepoFormattedLog(5, 2))
					data.CheckAppInterfaceCommitStats(0, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 2, 2)

					data.CheckAppInterfaceCommitMessage(1, data.GetTestRepoFormattedLog(7, 4, 1))
					data.CheckAppInterfaceCommitStats(1, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 2, 2)
				})
			})

			When("targets don't share the same git hash", func() {
				BeforeEach(func() {
					properties := InitProperties(data.TestRepoPath, "")
					properties["gitHashProd1Target1"] = data.TestRepoHashes[0]
					properties["gitHashProd1Target2"] = data.TestRepoHashes[1]
					properties["gitHashProd2Target1"] = data.TestRepoHashes[2]
					properties["gitHashProd2Target2"] = data.TestRepoHashes[3]

					data.WriteAppInterfaceFile("data/services/gen-app/cicd/saas/service-1.yaml", GetFileContent(ServiceFileContentTemplate, "service-1", properties))
					data.CommitAppInterfaceChanges("Setup different git hashes for targets")
				})

				It("promotes all targets in all resource templates", func() {
					err := service.Promote(&DefaultPromoteCallbacks{service}, data.TestRepoHashes[7])
					Expect(err).ShouldNot(HaveOccurred())

					Expect(data.GetAppInterfaceCommitsCount()).To(Equal(6))

					data.CheckAppInterfaceCommitMessage(0, data.GetTestRepoFormattedLog(5))
					data.CheckAppInterfaceCommitStats(0, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 1, 1)

					data.CheckAppInterfaceCommitMessage(1, data.GetTestRepoFormattedLog(5))
					data.CheckAppInterfaceCommitStats(1, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 1, 1)

					data.CheckAppInterfaceCommitMessage(2, data.GetTestRepoFormattedLog(7, 4))
					data.CheckAppInterfaceCommitStats(2, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 1, 1)

					data.CheckAppInterfaceCommitMessage(3, data.GetTestRepoFormattedLog(7, 4))
					data.CheckAppInterfaceCommitStats(3, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 1, 1)
				})
			})

			When("the promoted git hash is an ancestor of the current hash", func() {
				BeforeEach(func() {
					properties := InitProperties(data.TestRepoPath, data.TestRepoHashes[9])

					data.WriteAppInterfaceFile("data/services/gen-app/cicd/saas/service-1.yaml", GetFileContent(ServiceFileContentTemplate, "service-1", properties))
					data.CommitAppInterfaceChanges("Setup a more recent git hash for targets")
				})

				It("promotes all targets in all resource templates", func() {
					err := service.Promote(&DefaultPromoteCallbacks{service}, data.TestRepoHashes[7])
					Expect(err).ShouldNot(HaveOccurred())

					Expect(data.GetAppInterfaceCommitsCount()).To(Equal(4))

					data.CheckAppInterfaceCommitMessage(0, "```\n\n```")
					data.CheckAppInterfaceCommitStats(0, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 2, 2)

					data.CheckAppInterfaceCommitMessage(1, "```\n\n```")
					data.CheckAppInterfaceCommitStats(1, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 2, 2)
				})
			})

			When("using branch names", func() {
				BeforeEach(func() {
					testRepo, err := git.PlainOpen(data.TestRepoPath)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(testRepo).ToNot(BeNil())

					testWorkTree, err := testRepo.Worktree()
					Expect(err).ShouldNot(HaveOccurred())
					Expect(testWorkTree).NotTo(BeNil())

					err = testWorkTree.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("new"), Create: true})
					Expect(err).ShouldNot(HaveOccurred())
					err = testWorkTree.Reset(&git.ResetOptions{Mode: git.HardReset, Commit: plumbing.NewHash(data.TestRepoHashes[7])})
					Expect(err).ShouldNot(HaveOccurred())

					err = testWorkTree.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("current"), Create: true})
					Expect(err).ShouldNot(HaveOccurred())
					err = testWorkTree.Reset(&git.ResetOptions{Mode: git.HardReset, Commit: plumbing.NewHash(data.TestRepoHashes[0])})
					Expect(err).ShouldNot(HaveOccurred())

					err = testWorkTree.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("master")})
					Expect(err).ShouldNot(HaveOccurred())

					properties := InitProperties(data.TestRepoPath, "current")

					data.WriteAppInterfaceFile("data/services/gen-app/cicd/saas/service-1.yaml", GetFileContent(ServiceFileContentTemplate, "service-1", properties))
					data.CommitAppInterfaceChanges("Setup a more recent git hash for targets")
				})

				It("promotes all targets in all resource templates", func() {
					err := service.Promote(&DefaultPromoteCallbacks{service}, data.TestRepoHashes[7])
					Expect(err).ShouldNot(HaveOccurred())

					Expect(data.GetAppInterfaceCommitsCount()).To(Equal(4))

					data.CheckAppInterfaceCommitMessage(0, data.GetTestRepoFormattedLog(5, 2))
					data.CheckAppInterfaceCommitStats(0, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 2, 2)

					data.CheckAppInterfaceCommitMessage(1, data.GetTestRepoFormattedLog(7, 4, 1))
					data.CheckAppInterfaceCommitStats(1, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 2, 2)
				})
			})

			AfterEach(func() {
				data.CheckAppInterfaceService1Content(ServiceFileContentTemplate, InitProperties(data.TestRepoPath, data.TestRepoHashes[7]))
				data.CheckAppInterfaceIsClean()
				data.CheckAppInterfaceBranchName(fmt.Sprintf("promote-service-1-%s", data.TestRepoHashes[7]))
			})
		})

		When("using custom callbacks to consider hivep01 targets only", func() {
			It("only promotes those hivep01 targets", func() {
				err := service.Promote(&customPromoteCallbacks{DefaultPromoteCallbacks{service}, "hivep01"}, data.TestRepoHashes[6])
				Expect(err).ShouldNot(HaveOccurred())

				expectedProperties := InitProperties(data.TestRepoPath, data.TestRepoHashes[0])
				expectedProperties["gitHashProd1Target1"] = data.TestRepoHashes[6]
				expectedProperties["gitHashProd2Target1"] = data.TestRepoHashes[6]
				data.CheckAppInterfaceService1Content(ServiceFileContentTemplate, expectedProperties)

				data.CheckAppInterfaceIsClean()
				data.CheckAppInterfaceBranchName(fmt.Sprintf("promote-service-1-%s", data.TestRepoHashes[6]))

				Expect(data.GetAppInterfaceCommitsCount()).To(Equal(3))

				data.CheckAppInterfaceCommitMessage(0, data.GetTestRepoFormattedLog(5, 2))
				data.CheckAppInterfaceCommitStats(0, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 1, 1)

				data.CheckAppInterfaceCommitMessage(1, data.GetTestRepoFormattedLog(4, 1))
				data.CheckAppInterfaceCommitStats(1, 1, "data/services/gen-app/cicd/saas/service-1.yaml", 1, 1)
			})
		})

	})
})
