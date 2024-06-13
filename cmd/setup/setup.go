package setup

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	ProdJumproleConfigKey  = "prod_jumprole_account_id"
	AwsProxy               = "aws_proxy"
	StageJumproleConfigKey = "stage_jumprole_account_id"
	Pd_User_Token          = "pd_user_token"
	Jira_Token             = "jira_token"
	Dt_Vault_Path          = "dt_vault_path"
	Vault_Address          = "vault_address"
	JiraTokenRegex         = "^[A-Z0-9]{7}$"
	PdTokenRegex           = "^[a-zA-Z0-9+_-]{20}$"
	AwsAccountRegex        = "^[0-9]{12}$"
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
				Pd_User_Token,
				Jira_Token,
			}

			optionalKeys := []string{
				Dt_Vault_Path,
				Vault_Address,
			}

			values := make(map[string]string)
			reader := bufio.NewReader(os.Stdin)

			for _, key := range keys {
				fmt.Printf("Enter %s: ", key)
				value, _ := reader.ReadString('\n')
				value = strings.TrimSpace(value)

				var err error
				switch key {
				case Jira_Token:
					_, err = ValidateJiraToken(value)
				case Pd_User_Token:
					_, err = ValidatePDToken(value)
				case ProdJumproleConfigKey, AwsProxy, StageJumproleConfigKey:
					_, err = ValidateAWSAccount(value)
				}

				if err != nil {
					return err
				}

				values[key] = value
			}

			for _, key := range optionalKeys {
				fmt.Printf("Enter %s (optional): ", key)
				value, _ := reader.ReadString('\n')
				values[key] = strings.TrimSpace(value)
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
		return "", fmt.Errorf("invalid jira token")
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
		return "", fmt.Errorf("invalid pd token")
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
		return "", fmt.Errorf("invalid AWS account number")
	}
	return account, nil
}
