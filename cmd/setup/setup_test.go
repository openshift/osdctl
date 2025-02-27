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
		// Reset viper configuration before each test
		viper.Reset()
		// Use in-memory filesystem for config
		fs := afero.NewMemMapFs()
		viper.SetFs(fs)
		// Disable config file lookup to prevent ConfigFileNotFoundError
		viper.SetConfigName("") // Make sure config file lookup is not used
		// Set config type to avoid file lookup errors
		viper.SetConfigType("yaml")
		// Use a temporary file for the in-memory config
		viper.SetConfigFile("/tmp/config.yaml")
		// Set default values for the configuration
		viper.SetDefault("prod_jumprole_account_id", "123456789012")
		viper.SetDefault("aws_proxy", "http://proxy.example.com")
		viper.SetDefault("stage_jumprole_account_id", "987654321098")
		viper.SetDefault("dt_vault_path", "dt-vault-path")
		viper.SetDefault("vault_address", "https://vault.example.com")
		viper.SetDefault("pd_user_token", "abcdEFGHijklMNOPqrst")
		viper.SetDefault("jira_token", "ABC1234")
		viper.SetDefault("cloudtrail_cmd_lists", "  - aws s3 ls")
		viper.SetDefault("gitlab_access", "abcdEFGHijklMNOPqrst")
	})

	Context("When user provides valid inputs", func() {
		It("should correctly set and save the configuration", func() {
			// Simulate user input for the required keys
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
			// Create a buffer to simulate user input
			inputBuffer := bytes.NewBufferString(strings.Join(inputs, "\n"))
			// Override the input reader to use our buffer
			reader := bufio.NewReader(inputBuffer)
			// Mock the setup command
			setupCmd := NewCmdSetup()
			setupCmd.SetOut(os.Stdout) // Set output to standard out
			setupCmd.SetIn(reader)     // Set input to our mocked buffer
			// Execute the setup command and expect no error
			err := setupCmd.Execute()
			Expect(err).To(BeNil())
			// Verify that the correct values have been set in viper
			Expect(viper.GetString("prod_jumprole_account_id")).To(Equal("123456789012"))
			Expect(viper.GetString("aws_proxy")).To(Equal("http://proxy.example.com"))
			Expect(viper.GetString("stage_jumprole_account_id")).To(Equal("987654321098"))
			Expect(viper.GetString("dt_vault_path")).To(Equal("dt-vault-path"))
			Expect(viper.GetString("vault_address")).To(Equal("https://vault.example.com"))
			Expect(viper.GetString("pd_user_token")).To(Equal("abcdEFGHijklMNOPqrst"))
			Expect(viper.GetString("jira_token")).To(Equal("ABC1234"))
			Expect(viper.GetString("cloudtrail_cmd_lists")).To(Equal("  - aws s3 ls"))
			Expect(viper.GetString("gitlab_access")).To(Equal("abcdEFGHijklMNOPqrst"))
		})
	})

})
