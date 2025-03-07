package setup

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

func TestSetup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Setup Suite")
}

var _ = Describe("Validation Functions", func() {
	Context("Jira Token", func() {
		It("should validate correct Jira token", func() {
			token, _ := ValidateJiraToken("ABC1234")
			Expect(token).To(Equal("ABC1234"))
		})

		It("should fail invalid Jira token", func() {
			_, err := ValidateJiraToken("invalid")
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
			_, err := ValidatePDToken("short")
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
			_, err := ValidateAWSAccount("invalid123")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("AWS Proxy", func() {
		It("should validate the correct AWS proxy", func() {
			proxyURL, err := ValidateAWSProxy("http://www.example.com:1234")
			Expect(err).To(BeNil())
			Expect(proxyURL).To(Equal("http://www.example.com:1234"))
		})

		It("should fail invalid proxy URL", func() {
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
			vaultPath, err := ValidateDtVaultPath("osd-sre/dynatrace/sd-sre-grail-logs")
			Expect(err).To(BeNil())
			Expect(vaultPath).To(Equal("osd-sre/dynatrace/sd-sre-grail-logs"))
		})

		It("should fail invalid vault path", func() {
			_, err := ValidateDtVaultPath("/osd-sre/dynatrace/sd-sre-grail-logs/logs")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("CloudTrail Cmd Lists", func() {
		It("should validate correct CloudTrail command list", func() {
			cmd, err := ValidateCloudTrailCmdLists("  - aws s3 ls")
			Expect(err).To(BeNil())
			Expect(cmd).To(Equal("- aws s3 ls"))
		})

		It("should fail invalid CloudTrail command list", func() {
			_, err := ValidateCloudTrailCmdLists("invalid command")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("GitLab Token", func() {
		It("should validate correct GitLab token", func() {
			token, err := ValidateGitLabToken("abcdEFGHijklMNOPqrst")
			Expect(err).To(BeNil())
			Expect(token).To(Equal("abcdEFGHijklMNOPqrst"))
		})

		It("should fail invalid GitLab token", func() {
			_, err := ValidateGitLabToken("shorttoken")
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = Describe("NewCmdSetup Command", func() {
	BeforeEach(func() {
		viper.Reset()
		fs := afero.NewMemMapFs()
		viper.SetFs(fs)
		viper.SetConfigName("")
		viper.SetConfigType("yaml")
		viper.SetConfigFile("/tmp/config.yaml")

		for k, v := range map[string]string{
			ProdJumproleConfigKey:  "123456789012",
			AwsProxy:               "http://proxy.example.com",
			StageJumproleConfigKey: "987654321098",
			DtVaultPath:            "dt-vault-path",
			VaultAddress:           "https://vault.example.com",
			PdUserToken:            "abcdEFGHijklMNOPqrst",
			JiraToken:              "ABC1234",
			CloudTrailCmdLists:     "  - aws s3 ls",
			GitLabToken:            "abcdEFGHijklMNOPqrst",
		} {
			viper.SetDefault(k, v)
		}
	})

	Context("When user provides valid inputs", func() {
		It("should correctly set and save the configuration", func() {
			inputs := []string{
				"123456789012",              // ProdJumproleConfigKey
				"http://proxy.example.com",  // AwsProxy
				"987654321098",              // StageJumproleConfigKey
				"dt-vault-path",             // DtVaultPath (optional)
				"https://vault.example.com", // VaultAddress (optional)
				"abcdEFGHijklMNOPqrst",      // PdUserToken (optional)
				"ABC1234",                   // JiraToken (optional)
				"  - aws s3 ls",             // CloudTrailCmdLists (optional)
				"abcdEFGHijklMNOPqrst",      // GitLabToken (optional)
			}
			inputBuffer := bytes.NewBufferString(strings.Join(inputs, "\n"))
			reader := bufio.NewReader(inputBuffer)
			setupCmd := NewCmdSetup()
			setupCmd.SetOut(os.Stdout)
			setupCmd.SetIn(reader)
			err := setupCmd.Execute()
			Expect(err).To(BeNil())
			Expect(viper.GetString(ProdJumproleConfigKey)).To(Equal("123456789012"))
			Expect(viper.GetString(AwsProxy)).To(Equal("http://proxy.example.com"))
			Expect(viper.GetString(StageJumproleConfigKey)).To(Equal("987654321098"))
			Expect(viper.GetString(DtVaultPath)).To(Equal("dt-vault-path"))
			Expect(viper.GetString(VaultAddress)).To(Equal("https://vault.example.com"))
			Expect(viper.GetString(PdUserToken)).To(Equal("abcdEFGHijklMNOPqrst"))
			Expect(viper.GetString(JiraToken)).To(Equal("ABC1234"))
			Expect(viper.GetString(CloudTrailCmdLists)).To(Equal("  - aws s3 ls"))
			Expect(viper.GetString(GitLabToken)).To(Equal("abcdEFGHijklMNOPqrst"))
		})
	})

	Context("When user provides invalid inputs", func() {
		It("should fail if required inputs are not provided", func() {
			inputs := []string{
				"",                          // ProdJumproleConfigKey (required, but empty)
				"http://proxy.example.com",  // AwsProxy
				"987654321098",              // StageJumproleConfigKey
				"dt-vault-path",             // DtVaultPath (optional)
				"https://vault.example.com", // VaultAddress (optional)
				"abcdEFGHijklMNOPqrst",      // PdUserToken (optional)
				"ABC1234",                   // JiraToken (optional)
				"  - aws s3 ls",             // CloudTrailCmdLists (optional)
				"abcdEFGHijklMNOPqrst",      // GitLabToken (optional)
			}
			inputBuffer := bytes.NewBufferString(strings.Join(inputs, "\n"))
			reader := bufio.NewReader(inputBuffer)
			setupCmd := NewCmdSetup()
			setupCmd.SetOut(os.Stdout)
			setupCmd.SetIn(reader)
			err := setupCmd.Execute()
			Expect(err).To(HaveOccurred())
		})

		It("should fail if invalid AWS account is provided", func() {
			inputs := []string{
				"invalid-account",           // ProdJumproleConfigKey (invalid)
				"http://proxy.example.com",  // AwsProxy
				"987654321098",              // StageJumproleConfigKey
				"dt-vault-path",             // DtVaultPath (optional)
				"https://vault.example.com", // VaultAddress (optional)
				"abcdEFGHijklMNOPqrst",      // PdUserToken (optional)
				"ABC1234",                   // JiraToken (optional)
				"  - aws s3 ls",             // CloudTrailCmdLists (optional)
				"abcdEFGHijklMNOPqrst",      // GitLabToken (optional)
			}
			inputBuffer := bytes.NewBufferString(strings.Join(inputs, "\n"))
			reader := bufio.NewReader(inputBuffer)
			setupCmd := NewCmdSetup()
			setupCmd.SetOut(os.Stdout)
			setupCmd.SetIn(reader)
			err := setupCmd.Execute()
			Expect(err).To(HaveOccurred())
		})
	})
})
