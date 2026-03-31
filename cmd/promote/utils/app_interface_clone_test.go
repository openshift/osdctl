package utils

import (
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("AppInterfaceClone struct", func() {
	var data *TestData

	BeforeEach(func() {
		data = CreateDefaultTestData()
	})

	AfterEach(func() {
		CleanupAllTestDataResources()
	})

	Context("FindAppInterfaceClone method", func() {
		var appInterfaceClone *AppInterfaceClone
		var callErr error

		When("app-interface clone path provided", func() {
			It("succeeds if called with an app-interface clone path", func() {
				appInterfaceClone, callErr = FindAppInterfaceClone(data.AppInterfacePath)
			})

			It("succeeds if current working directory is an app-interface clone", func() {
				err := os.Chdir(data.AppInterfacePath)
				Expect(err).ShouldNot(HaveOccurred())

				appInterfaceClone, callErr = FindAppInterfaceClone("")
			})

			It("succeeds if current working directory is in an app-interface clone", func() {
				err := os.Chdir(filepath.Join(data.AppInterfacePath, "data"))
				Expect(err).ShouldNot(HaveOccurred())

				appInterfaceClone, callErr = FindAppInterfaceClone("")
			})

			AfterEach(func() {
				Expect(callErr).ShouldNot(HaveOccurred())
				Expect(appInterfaceClone).ToNot(BeNil())
				Expect(appInterfaceClone.path).To(Equal(data.AppInterfacePath))
				Expect(appInterfaceClone.GetPath()).To(Equal(data.AppInterfacePath))
			})
		})

		When("no app-interface clone path provided", func() {
			It("fails if called with a path which is not a git clone", func() {
				appInterfaceClone, callErr = FindAppInterfaceClone(os.TempDir())
			})

			It("fails if called with a path which is a git clone but not an app-interface clone", func() {
				appInterfaceClone, callErr = FindAppInterfaceClone(data.TestRepoPath)
			})

			It("fails if current working directory is not an app-interface clone nor in one", func() {
				err := os.Chdir(data.TestRepoPath)
				Expect(err).ShouldNot(HaveOccurred())

				appInterfaceClone, callErr = FindAppInterfaceClone("")
			})

			It("fails if current working directory is an app-interface clone but is called with a path which is not an app-interface clone", func() {
				err := os.Chdir(data.AppInterfacePath)
				Expect(err).ShouldNot(HaveOccurred())

				appInterfaceClone, callErr = FindAppInterfaceClone(data.TestRepoPath)
			})

			AfterEach(func() {
				Expect(callErr).Should(HaveOccurred())
				Expect(appInterfaceClone).To(BeNil())
			})
		})
	})

	Context("CheckoutNewBranch method", func() {
		It("succeeds if called with a branch name which does not exist in the clone", func() {
		})

		It("succeeds if called with a branch name which already exist in the clone", func() {
			// Creating the branch
			appInterfaceRepo, err := git.PlainOpen(data.AppInterfacePath)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(appInterfaceRepo).ToNot(BeNil())

			appInterfaceWorkTree, err := appInterfaceRepo.Worktree()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(appInterfaceWorkTree).ToNot(BeNil())

			err = appInterfaceWorkTree.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("test-branch"), Create: true})
			Expect(err).ShouldNot(HaveOccurred())

		})

		AfterEach(func() {
			appInterfaceClone, err := FindAppInterfaceClone(data.AppInterfacePath)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(appInterfaceClone).ToNot(BeNil())

			// Making the call
			err = appInterfaceClone.CheckoutNewBranch("test-branch")
			Expect(err).ShouldNot(HaveOccurred())

			data.CheckAppInterfaceIsClean()
			data.CheckAppInterfaceBranchName("test-branch")
			data.CheckAppInterfaceCommitMessage(0, "Initial commit")
		})
	})
})
