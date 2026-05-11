package managedscripts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/openshift/osdctl/pkg/promote"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var serviceFileContentBackplaneTemplate = `name: saas-backplane-api
app:
  $ref: /services/backplane/app.yaml
resourceTemplates:
- name: backplane-api
  url: @prod1repoUrl@
  path: /templates/prod1.yaml
  targets:
  - namespace:
      $ref: /services/backplane/namespaces/backplane-stage-backplanes01.yml
    ref: master
  - namespace:
      $ref: /services/backplane/namespaces/backplane-prod-backplanep01.yml
    ref: @gitHashProd1Target1@
    parameters:
      MANAGED_SCRIPTS_GIT_SHA: @managedScriptsGitHash@
  - namespace:
      $ref: /services/backplane/namespaces/backplane-prod-backplanep01.yml
    ref: @gitHashProd1Target2@
    parameters:
      MANAGED_SCRIPTS_GIT_SHA: @managedScriptsGitHash@
`

func TestSetup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Setup Suite")
}

type managedScriptsTestData struct {
	*promote.TestData

	managedScriptsRepoPath   string
	managedScriptsRepoHashes [10]string
}

func CreateManagedScriptsTestData(nestedData *promote.TestData) *managedScriptsTestData {
	data := managedScriptsTestData{
		TestData: nestedData,

		managedScriptsRepoPath: filepath.Join(filepath.Dir(nestedData.TestRepoPath), "managed-scripts"),
	}

	err := os.Mkdir(data.managedScriptsRepoPath, 0700)
	Expect(err).ShouldNot(HaveOccurred())

	managedscriptsRepo, err := git.PlainInit(data.managedScriptsRepoPath, false)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(managedscriptsRepo).NotTo(BeNil())

	managedscriptsWorkTree, err := managedscriptsRepo.Worktree()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(managedscriptsWorkTree).NotTo(BeNil())

	templatePath := filepath.Join(data.managedScriptsRepoPath, templateRelPath)
	templatesDirPath := filepath.Dir(templatePath)
	err = os.MkdirAll(templatesDirPath, 0700)
	Expect(err).ShouldNot(HaveOccurred())

	for k := 0; k < 10; k++ {
		if k%2 == 0 {
			err = os.WriteFile(templatePath, []byte(fmt.Sprintf("%d", k)), 0600)
			Expect(err).ShouldNot(HaveOccurred())

			err := managedscriptsWorkTree.AddGlob(".")
			Expect(err).ShouldNot(HaveOccurred())
		}

		hash, err := managedscriptsWorkTree.Commit(fmt.Sprintf("Commit #%d", k), &git.CommitOptions{
			Author:            &promote.DefaultSignature,
			AllowEmptyCommits: true,
		})
		Expect(err).ShouldNot(HaveOccurred())

		data.managedScriptsRepoHashes[k] = hash.String()
	}

	return &data
}

func (d *managedScriptsTestData) GetManagedScriptsRepoFormattedLog(hashIndexes ...int) string {
	var sb strings.Builder

	for _, idx := range hashIndexes {
		fmt.Fprintf(&sb, promote.CommitTemplate, d.managedScriptsRepoHashes[idx], idx)
	}

	return sb.String()
}

type promoteCallbacksMock struct {
	promoteCallbacks

	data *managedScriptsTestData
}

func (c *promoteCallbacksMock) GetResourceTemplateRepoUrl(*kyaml.RNode) (string, error) {
	return c.data.managedScriptsRepoPath, nil
}

var _ = Describe("Service struct", func() {
	var data *managedScriptsTestData
	var service *promote.Service

	BeforeEach(func() {
		var properties map[string]string

		data = CreateManagedScriptsTestData(promote.CreateTestData(func(data *promote.TestData) map[string]string {
			properties = promote.InitProperties(data.TestRepoPath, data.TestRepoHashes[1])
			return map[string]string{
				"data/services/backplane/app.yaml": promote.GetFileContent(promote.AppFileContentTemplate, "backplane", properties),
			}
		}))

		properties["managedScriptsGitHash"] = data.managedScriptsRepoHashes[2]

		data.WriteAppInterfaceFile(serviceRelPath, promote.GetFileContent(serviceFileContentBackplaneTemplate, "", properties))
		data.CommitAppInterfaceChanges("Defining the service to promote")
	})

	JustBeforeEach(func() {
		var err error

		appInterfaceClone, err := promote.FindAppInterfaceClone(data.AppInterfacePath)
		Expect(err).ShouldNot(HaveOccurred())
		service, err = promote.ReadServiceFromFile(
			appInterfaceClone,
			filepath.Join(appInterfaceClone.GetPath(), serviceRelPath))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(service).ToNot(BeNil())
	})

	AfterEach(func() {
		promote.CleanupAllTestDataResources()
	})

	Context("Promote method", func() {
		When("namespaceRef is set to 'hivep'", func() {
			It("promotes all targets in all resource templates", func() { // because all namespaces have their ref contain that string
				err := service.Promote(&promoteCallbacksMock{
					promoteCallbacks: promoteCallbacks{DefaultPromoteCallbacks: promote.DefaultPromoteCallbacks{Service: service}},
					data:             data,
				}, data.managedScriptsRepoHashes[8])
				Expect(err).ShouldNot(HaveOccurred())

				expectedProperties := promote.InitProperties(data.TestRepoPath, data.TestRepoHashes[1])
				expectedProperties["managedScriptsGitHash"] = data.managedScriptsRepoHashes[8]

				data.CheckAppInterfaceFileContent(serviceRelPath, serviceFileContentBackplaneTemplate, "", expectedProperties)

				data.CheckAppInterfaceIsClean()
				data.CheckAppInterfaceBranchName(fmt.Sprintf("promote-saas-backplane-api-%s", data.managedScriptsRepoHashes[8]))

				Expect(data.GetAppInterfaceCommitsCount()).To(Equal(3))

				data.CheckAppInterfaceCommitMessage(0, data.GetManagedScriptsRepoFormattedLog(8, 6, 4))
				data.CheckAppInterfaceCommitStats(0, 1, serviceRelPath, 2, 2)
			})
		})
	})
})
