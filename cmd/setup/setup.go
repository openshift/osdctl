package setup

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	ProdJumproleConfigKey   = "prod_jumprole_account_id"
	AwsProxy                = "aws_proxy"
	StageJumproleConfigKey  = "stage_jumprole_account_id"
	PdUserToken             = "pd_user_token"
	JiraToken               = "jira_token"
	DtVaultPath             = "dt_vault_path"
	VaultAddress            = "vault_address"
	CloudTrailCmdLists      = "cloudtrail_cmd_lists"
	GitLabToken             = "gitlab_access"
	JiraTokenRegex          = "^[A-Z0-9]{7}$"
	PdTokenRegex            = "^[a-zA-Z0-9+_-]{20}$"
	AwsAccountRegex         = "^[0-9]{12}$"
	AWSProxyRegex           = `^http:\/\/[a-zA-Z0-9.-]+(:\d+)?$`
	VaultURLRegex           = `^https:\/\/[a-zA-Z0-9.-]+\/?$`
	DtVaultPathRegex        = `^[a-zA-Z0-9\-/]+$`
	CloudTrailCmdListsRegex = `^\s*-\s+.*$`
	GitLabTokenRegex        = `^[a-zA-Z0-9]{20}$`
)

// NewCmdSetup implements the setup command
func NewCmdSetup() *cobra.Command {
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Setup the configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys := []string{
				ProdJumproleConfigKey,
				AwsProxy,
				StageJumproleConfigKey,
			}

			optionalKeys := []string{
				DtVaultPath,
				VaultAddress,
				PdUserToken,
				JiraToken,
				CloudTrailCmdLists,
				GitLabToken,
			}

			values := make(map[string]string)
			reader := bufio.NewReader(os.Stdin)

			defaults := make(map[string]string)
			for _, key := range keys {
				defaultValue := viper.GetString(key)
				defaults[key] = defaultValue
			}

			for _, key := range optionalKeys {
				defaultValue := viper.GetString(key)
				defaults[key] = defaultValue
			}

			for _, key := range keys {
				defaultValue := defaults[key]
				fmt.Printf("\033[91mEnter %s \033[0m [\033[94mdefault %s\033[0m]:", key, defaultValue)
				value, _ := reader.ReadString('\n')
				value = strings.TrimSpace(value)

				if value == "" {
					value = defaultValue
				}

				var err error
				switch key {
				case JiraToken:
					if value != "" && value != defaultValue {
						_, err = ValidateJiraToken(value)
					}
				case PdUserToken:
					if value != "" && value != defaultValue {
						_, err = ValidatePDToken(value)
					}
				case ProdJumproleConfigKey, StageJumproleConfigKey:
					if value != "" && value != defaultValue {
						_, err = ValidateAWSAccount(value)
					}
				case AwsProxy:
					if value != "" && value != defaultValue {
						_, err = ValidateAWSProxy(value)
					}
				case VaultAddress:
					if value != "" && value != defaultValue {
						_, err = ValidateVaultAddress(value)
					}
				case DtVaultPath:
					if value != "" && value != defaultValue {
						_, err = ValidateDtVaultPath(value)
					}
				case CloudTrailCmdLists:
					if value != "" && value != defaultValue {
						_, err = ValidateCloudTrailCmdLists(value)
					}
				case GitLabToken:
					if value != "" && value != defaultValue {
						_, err = ValidateGitLabToken(value)
					}
				}

				if err != nil {
					return err
				}

				values[key] = value
			}

			for _, key := range optionalKeys {
				defaultValue := defaults[key]
				fmt.Printf("\033[91mEnter %s (optional)\033[0m [\033[94mdefault %s\033[0m]:", key, defaultValue)
				value, _ := reader.ReadString('\n')
				value = strings.TrimSpace(value)

				if value == "" {
					value = defaultValue
				}

				if value != "" && value != defaultValue {
					values[key] = value
				}
			}

			// Store the value in the config file
			for key, value := range values {
				viper.Set(key, value)
			}
			err := viper.WriteConfig()
			if err != nil {
				return err
			}

			fmt.Println("Configuration saved successfully")
			return nil
		},
	}
	return setupCmd
}

func ValidateJiraToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	match, err := regexp.MatchString(JiraTokenRegex, token)
	if err != nil {
		return "", err
	}
	if !match {
		return "", errors.New("invalid jira token")
	}
	return token, nil
}

func ValidatePDToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	match, err := regexp.MatchString(PdTokenRegex, token)
	if err != nil {
		return "", err
	}
	if !match {
		return "", errors.New("invalid pd token")
	}
	return token, nil
}

func ValidateAWSAccount(account string) (string, error) {
	account = strings.TrimSpace(account)
	match, err := regexp.MatchString(AwsAccountRegex, account)
	if err != nil {
		return "", err
	}
	if !match {
		return "", errors.New("invalid AWS account number")
	}
	return account, nil
}

func ValidateAWSProxy(proxyURL string) (string, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	match, err := regexp.MatchString(AWSProxyRegex, proxyURL)
	if err != nil {
		return "", err
	}
	if !match {
		return "", errors.New("invalid AWS proxy URL")
	}
	return proxyURL, nil
}

func ValidateVaultAddress(vaultURL string) (string, error) {
	vaultURL = strings.TrimSpace(vaultURL)
	match, err := regexp.MatchString(VaultURLRegex, vaultURL)
	if err != nil {
		return "", err
	}
	if !match {
		return "", errors.New("invalid Vault URL")
	}
	return vaultURL, nil
}

func ValidateDtVaultPath(dtVaultPath string) (string, error) {
	dtVaultPath = strings.TrimSpace(dtVaultPath)
	match, err := regexp.MatchString(DtVaultPathRegex, dtVaultPath)
	if err != nil {
		return "", err
	}
	if !match {
		return "", errors.New("invalid DtVault Path")
	}
	return dtVaultPath, nil
}

func ValidateCloudTrailCmdLists(cloudTrailCmd string) (string, error) {
	cloudTrailCmd = strings.TrimSpace(cloudTrailCmd)
	match, err := regexp.MatchString(CloudTrailCmdListsRegex, cloudTrailCmd)
	if err != nil {
		return "", err
	}
	if !match {
		return "", errors.New("invalid CloudTrail command")
	}
	return cloudTrailCmd, nil
}

func ValidateGitLabToken(GitLabtoken string) (string, error) {
	GitLabtoken = strings.TrimSpace(GitLabtoken)
	match, err := regexp.MatchString(GitLabTokenRegex, GitLabtoken)
	if err != nil {
		return "", err
	}
	if !match {
		return "", errors.New("invalid GitLab token")
	}
	return GitLabtoken, nil
}
