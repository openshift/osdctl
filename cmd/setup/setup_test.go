package setup

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"testing"
)

func TestSetup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Setup Suite")
}

var _ = Describe("Validation Functions", func() {
	Context("Jira Token", func() {
		It("should validate correct Jira token", func() {
			token, err := ValidateJiraToken("ABC1234")
			Expect(err).To(BeNil())
			Expect(token).To(Equal("ABC1234"))
		})

		It("should fail invalid Jira token", func() {
			_, err := ValidateJiraToken("abc") // this should fail since "INVALID" does not match ^[A-Z0-9]{7}$
			Expect(err).To(HaveOccurred())
		})
	})

	Context("PD Token", func() {
		It("should validate correct PD token", func() {
			token, err := ValidatePDToken("abcdEFGHijklMNOPqrst")
			Expect(err).To(BeNil())
			Expect(token).To(Equal("abcdEFGHijklMNOPqrst"))
		})

		It("should fail invalid PD token", func() {
			_, err := ValidatePDToken("short") // this should fail since "short" does not match ^[a-zA-Z0-9+_-]{20}$
			Expect(err).To(HaveOccurred())
		})
	})

	Context("AWS Account", func() {
		It("should validate correct AWS account", func() {
			account, err := ValidateAWSAccount("123456789012")
			Expect(err).To(BeNil())
			Expect(account).To(Equal("123456789012"))
		})

		It("should fail invalid AWS account", func() {
			_, err := ValidateAWSAccount("invalid123") // this should fail since "invalid123" does not match ^[0-9]{12}$
			Expect(err).To(HaveOccurred())
		})
	})
})
