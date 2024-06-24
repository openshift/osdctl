package setup

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSetup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Setup Suite")
}

var _ = Describe("Validation Functions", func() {
	Context("Jira Token", func() {
		It("should validate correct Jira token", func() {
			token, _ := ValidateJiraToken("ABC1234")
			//Expect(err).To(BeNil())
			Expect(token).To(Equal("ABC1234"))
		})

		It("should fail invalid Jira token", func() {
			_, err := ValidateJiraToken("invalid") // this should fail since "INVALID" does not match ^[A-Z0-9]{7}$
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

	Context("AWS Proxy", func() {
		It("should validate the correct aws proxy", func() {
			proxyURL, err := ValidateAWSProxy("http://www.example.com:1234")
			Expect(err).To(BeNil())
			Expect(proxyURL).To(Equal("http://www.example.com:1234"))
		})

		It("should fail invalid proxy url", func() {
			_, err := ValidateAWSProxy("https://www.example.com:1234")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Vault Address", func() {
		It("should validate the correct vault address", func() {
			vaultURL, err := ValidateVaultAddress("https://vault.dev.net/")
			Expect(err).To(BeNil())
			Expect(vaultURL).To(Equal("https://vault.dev.net/"))
		})

		It("should fail invalid vault address", func() {
			_, err := ValidateVaultAddress("http://vault.dev.net/")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Vault Path", func() {
		It("should validate the correct vault path", func() {
			proxyURL, err := ValidateDtVaultPath("osd-sre/dynatrace/sd-sre-grail-logs")
			Expect(err).To(BeNil())
			Expect(proxyURL).To(Equal("osd-sre/dynatrace/sd-sre-grail-logs"))
		})

		It("should fail invalid vault path", func() {
			_, err := ValidateDtVaultPath("/osd-sre/dynatrace/sd-sre-grail-logs/logs")
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})
