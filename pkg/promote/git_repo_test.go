package promote

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Repo.FormattedLog", func() {
	var data *TestData

	BeforeEach(func() {
		data = CreateDefaultTestData()
	})

	AfterEach(func() {
		CleanupAllTestDataResources()
	})

	It("returns all non-merge commits between two hashes regardless of which files they touch", func() {
		repo, err := GetRepo(data.TestRepoPath)
		Expect(err).ShouldNot(HaveOccurred())
		defer repo.Cleanup()

		log, err := repo.FormattedLog(data.TestRepoHashes[0], data.TestRepoHashes[7])
		Expect(err).ShouldNot(HaveOccurred())

		expectedLog := data.GetTestRepoFormattedLog(7, 6, 5, 4, 3, 2, 1)
		Expect(log).To(Equal(expectedLog))
	})

	It("returns an empty string when target is an ancestor of the common ancestor", func() {
		repo, err := GetRepo(data.TestRepoPath)
		Expect(err).ShouldNot(HaveOccurred())
		defer repo.Cleanup()

		log, err := repo.FormattedLog(data.TestRepoHashes[7], data.TestRepoHashes[3])
		Expect(err).ShouldNot(HaveOccurred())

		Expect(log).To(Equal(""))
	})
})
