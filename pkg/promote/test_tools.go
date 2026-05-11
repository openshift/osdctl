package promote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	. "github.com/onsi/gomega" //nolint:staticcheck
)

const (
	ServiceFileContentTemplate = `name: @name@
app:
  $ref: /services/gen-app/app.yml
resourceTemplates:
- name: stage
  url: @stageRepoUrl@
  path: /templates/stage.yaml
  targets:
  - name: hives01
    namespace:
      $ref: /services/gen-app/namespaces/hives01/cluster-scope.yml
    ref: master
- name: prod1
  url: @prod1repoUrl@
  path: /templates/prod1.yaml
  targets:
  - name: hivep01
    namespace:
      $ref: /services/gen-app/namespaces/hivep01/cluster-scope.yml
    ref: @gitHashProd1Target1@
  - name: hivep02
    namespace:
      $ref: /services/gen-app/namespaces/hivep02/cluster-scope.yml
    ref: @gitHashProd1Target2@
- name: prod2
  url: @prod2repoUrl@
  path: /templates/prod2.yaml
  targets:
  - name: hivep01
    namespace:
      $ref: /services/gen-app/namespaces/hivep01/cluster-scope.yml
    ref: @gitHashProd2Target1@
  - name: hivep02
    namespace:
      $ref: /services/gen-app/namespaces/hivep02/cluster-scope.yml
    ref: @gitHashProd2Target2@
`
	AppFileContentTemplate = `name: @name@
codeComponents:
- name: dummy-component
  resource: upstream
  url: https://github.com/openshift/dummy
- name: default-component
  resource: upstream
  url: @appRepoUrl@
`
	AppFileContentTemplateWithHotfixVersion = AppFileContentTemplate + `  hotfixVersions:
  - @hotfixVersion@
`
	AppFileContentTemplateWithHotfixVersions = AppFileContentTemplate + `  hotfixVersions:
  - @hotfixVersion1@
  - @hotfixVersion2@
`
	AppFileContentTemplateWithBlockedVersion = AppFileContentTemplate + `  blockedVersions:
  - @blockedVersion@
`
	AppFileContentTemplateWithBlockedVersions = AppFileContentTemplate + `  blockedVersions:
  - @blockedVersion1@
  - @blockedVersion2@
`
)

func GetFileContent(template string, name string, properties map[string]string) string {
	content := template

	for placeholder, value := range properties {
		content = strings.ReplaceAll(content, "@"+placeholder+"@", value)
	}

	return strings.ReplaceAll(content, "@name@", name)
}

type TestData struct {
	TestRepoPath     string
	TestRepoHashes   [10]string
	AppInterfacePath string
}

var DefaultSignature = object.Signature{
	Name:  "John Doe",
	Email: "john.doe@company.com",
}

var createdTestDataPaths []string

func CleanupAllTestDataResources() {
	for _, path := range createdTestDataPaths {
		err := os.RemoveAll(path)
		Expect(err).ShouldNot(HaveOccurred())
	}

	createdTestDataPaths = nil
}

func CreateTestData(getAppInterfaceContent func(data *TestData) map[string]string) *TestData {
	rootPath, err := os.MkdirTemp("", "cmd-promote-utils")
	Expect(err).ShouldNot(HaveOccurred())
	rootPath, err = filepath.EvalSymlinks(rootPath)
	Expect(err).ShouldNot(HaveOccurred())
	createdTestDataPaths = append(createdTestDataPaths, rootPath)

	data := TestData{
		TestRepoPath:     filepath.Join(rootPath, "test-repo"),
		AppInterfacePath: filepath.Join(rootPath, "app-interface"),
	}

	// Initializing test repo

	err = os.Mkdir(data.TestRepoPath, 0700)
	Expect(err).ShouldNot(HaveOccurred())

	testRepo, err := git.PlainInit(data.TestRepoPath, false)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(testRepo).NotTo(BeNil())

	testWorkTree, err := testRepo.Worktree()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(testWorkTree).NotTo(BeNil())

	templatesDirPath := filepath.Join(data.TestRepoPath, "templates")
	err = os.Mkdir(templatesDirPath, 0700)
	Expect(err).ShouldNot(HaveOccurred())

	for k := 0; k < 10; k++ {
		fileName := []string{"stage.yaml", "prod1.yaml", "prod2.yaml"}[k%3] //nolint:gosec
		filePath := filepath.Join(templatesDirPath, fileName)

		err = os.WriteFile(filePath, []byte(fmt.Sprintf("%d", k)), 0600)
		Expect(err).ShouldNot(HaveOccurred())

		err := testWorkTree.AddGlob(".")
		Expect(err).ShouldNot(HaveOccurred())

		hash, err := testWorkTree.Commit(fmt.Sprintf("Commit #%d", k), &git.CommitOptions{
			Author: &DefaultSignature,
		})
		Expect(err).ShouldNot(HaveOccurred())

		data.TestRepoHashes[k] = hash.String()
	}

	// Initializing app-interface clone

	err = os.Mkdir(data.AppInterfacePath, 0700)
	Expect(err).ShouldNot(HaveOccurred())

	appInterfaceRepo, err := git.PlainInit(data.AppInterfacePath, false)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(appInterfaceRepo).NotTo(BeNil())

	appInterfaceRepoConfig := config.NewConfig()
	Expect(appInterfaceRepoConfig).NotTo(BeNil())
	appInterfaceRepoConfig.Author.Name = DefaultSignature.Name
	appInterfaceRepoConfig.Author.Email = DefaultSignature.Email
	err = appInterfaceRepo.SetConfig(appInterfaceRepoConfig)
	Expect(err).ShouldNot(HaveOccurred())

	_, err = appInterfaceRepo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"git@gitlab.cee.redhat.com:service/app-interface.git"},
	})
	Expect(err).ShouldNot(HaveOccurred())

	for fileRelPath, fileContent := range getAppInterfaceContent(&data) {
		data.WriteAppInterfaceFile(fileRelPath, fileContent)
	}

	data.CommitAppInterfaceChanges("Initial commit")

	return &data
}

func InitProperties(repoUrl, gitHash string) map[string]string {
	return map[string]string{
		"stageRepoUrl":        repoUrl,
		"prod1repoUrl":        repoUrl,
		"prod2repoUrl":        repoUrl,
		"appRepoUrl":          repoUrl,
		"gitHashProd1Target1": gitHash,
		"gitHashProd1Target2": gitHash,
		"gitHashProd2Target1": gitHash,
		"gitHashProd2Target2": gitHash,
	}
}

func CreateDefaultTestData() *TestData {
	return CreateTestData(func(data *TestData) map[string]string {
		properties := InitProperties(data.TestRepoPath, data.TestRepoHashes[0])

		return map[string]string{
			"data/services/gen-app/cicd/saas/service-1.yaml": GetFileContent(ServiceFileContentTemplate, "service-1", properties),
			"data/services/gen-app/app.yml":                  GetFileContent(AppFileContentTemplate, "gen-app", properties),
		}
	})
}

const CommitTemplate = `commit %s
Author: John Doe <john.doe@company.com>
Date:   Thu Jan 01 00:00:00 1970 +0000

    Commit #%d
`

func (d *TestData) GetTestRepoFormattedLog(hashIndexes ...int) string {
	var sb strings.Builder

	for _, idx := range hashIndexes {
		fmt.Fprintf(&sb, CommitTemplate, d.TestRepoHashes[idx], idx)
	}

	return sb.String()
}

func (d *TestData) ReadAppInterfaceFile(fileRelPath string) string {
	filePath := filepath.Join(d.AppInterfacePath, fileRelPath)

	fileContent, err := os.ReadFile(filePath)
	Expect(err).ShouldNot(HaveOccurred())

	return string(fileContent)
}

func (d *TestData) WriteAppInterfaceFile(fileRelPath string, fileContent string) {
	filePath := filepath.Join(d.AppInterfacePath, fileRelPath)
	dirPath := filepath.Dir(filePath)

	err := os.MkdirAll(dirPath, 0700)
	Expect(err).ShouldNot(HaveOccurred())

	err = os.WriteFile(filePath, []byte(fileContent), 0600)
	Expect(err).ShouldNot(HaveOccurred())
}

func (d *TestData) getAppInterfaceRepo() *git.Repository {
	appInterfaceRepo, err := git.PlainOpen(d.AppInterfacePath)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(appInterfaceRepo).ToNot(BeNil())

	return appInterfaceRepo
}

func (d *TestData) CommitAppInterfaceChanges(commitMessage string) {
	appInterfaceRepo := d.getAppInterfaceRepo()

	appInterfaceWorkTree, err := appInterfaceRepo.Worktree()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(appInterfaceWorkTree).NotTo(BeNil())

	err = appInterfaceWorkTree.AddGlob("*")
	Expect(err).ShouldNot(HaveOccurred())

	_, err = appInterfaceWorkTree.Commit(commitMessage, &git.CommitOptions{})
	Expect(err).ShouldNot(HaveOccurred())
}

func (d *TestData) CheckAppInterfaceFileContent(fileRelPath, expectedTemplate, expectedName string, expectedProperties map[string]string) {
	actualFileContent := d.ReadAppInterfaceFile(fileRelPath)
	expectedFileContent := GetFileContent(expectedTemplate, expectedName, expectedProperties)
	Expect(actualFileContent).To(Equal(expectedFileContent), "EXPECTED:\n%s\nBUT GOT:\n%s", expectedFileContent, actualFileContent)
}

func (d *TestData) CheckAppInterfaceService1Content(expectedTemplate string, expectedProperties map[string]string) {
	d.CheckAppInterfaceFileContent("data/services/gen-app/cicd/saas/service-1.yaml", expectedTemplate, "service-1", expectedProperties)
}

func (d *TestData) CheckAppInterfaceIsClean() {
	appInterfaceRepo := d.getAppInterfaceRepo()

	appInterfaceWorkTree, err := appInterfaceRepo.Worktree()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(appInterfaceWorkTree).NotTo(BeNil())

	appInterfaceStatus, err := appInterfaceWorkTree.Status()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(appInterfaceStatus.IsClean()).To(BeTrue())
}

func (d *TestData) CheckAppInterfaceBranchName(expectedBranchName string) {
	appInterfaceRepo := d.getAppInterfaceRepo()

	appInterfaceHead, err := appInterfaceRepo.Head()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(appInterfaceHead).NotTo(BeNil())

	Expect(appInterfaceHead.Name().String()).To(Equal("refs/heads/" + expectedBranchName))

	appInterfaceHeadCommit, err := appInterfaceRepo.CommitObject(appInterfaceHead.Hash())
	Expect(err).ShouldNot(HaveOccurred())
	Expect(appInterfaceHeadCommit).NotTo(BeNil())
}

func (d *TestData) iterOnAppInterfaceCommits(commitCallback func(*object.Commit) error) {
	appInterfaceRepo := d.getAppInterfaceRepo()

	commitsIt, err := appInterfaceRepo.Log(&git.LogOptions{Order: git.LogOrderCommitterTime})
	Expect(err).ShouldNot(HaveOccurred())

	err = commitsIt.ForEach(commitCallback)
	Expect(err).ShouldNot(HaveOccurred())
}

func (d *TestData) GetAppInterfaceCommitsCount() int {
	count := 0
	d.iterOnAppInterfaceCommits(func(c *object.Commit) error {
		count++
		return nil
	})

	return count
}

func (d *TestData) GetAppInterfaceCommit(index int) *object.Commit {
	count := d.GetAppInterfaceCommitsCount()
	Expect(index).To(BeNumerically(">=", 0))
	Expect(index).To(BeNumerically("<", count))

	var commit *object.Commit
	currentIndex := 0

	d.iterOnAppInterfaceCommits(func(c *object.Commit) error {
		if currentIndex == index {
			commit = c
			return storer.ErrStop
		}

		currentIndex++
		return nil
	})
	Expect(commit).ToNot(BeNil())

	return commit
}

func (d *TestData) CheckAppInterfaceCommitMessage(commitIndex int, expectedSubString string) {
	commit := d.GetAppInterfaceCommit(commitIndex)
	Expect(commit.Message).To(ContainSubstring(expectedSubString))
}

func (d *TestData) CheckAppInterfaceCommitStats(commitIndex, filesCount int, fileRelPath string, addChangesCount, delChangesCount int) {
	commit := d.GetAppInterfaceCommit(commitIndex)

	stats, err := commit.Stats()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(stats).To(HaveLen(filesCount))

	for _, stat := range stats {
		if stat.Name == fileRelPath {
			Expect(stat.Addition).To(Equal(addChangesCount))
			Expect(stat.Deletion).To(Equal(delChangesCount))
			return
		}
	}
	statFound := false
	Expect(statFound).To(BeTrue(), fmt.Sprintf("unable to find a stat for file '%s' in commit '%s'", fileRelPath, commit.Hash.String()))
}

func CreateServiceRegistry(data *TestData, validateServiceFilePathCallback ValidateServiceFilePathCallback, servicesDirsRelPaths ...string) *ServicesRegistry {
	appInterfaceClone, err := FindAppInterfaceClone(data.AppInterfacePath)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(appInterfaceClone).ToNot(BeNil())

	servicesRegistry, err := NewServicesRegistry(
		appInterfaceClone,
		validateServiceFilePathCallback,
		servicesDirsRelPaths...)

	Expect(err).ShouldNot(HaveOccurred())
	Expect(servicesRegistry).ToNot(BeNil())

	return servicesRegistry
}

func CreateDefaultServiceRegistry(data *TestData) *ServicesRegistry {
	return CreateServiceRegistry(data, func(filePath string) string { return filePath }, "data/services/gen-app/cicd/saas")
}
