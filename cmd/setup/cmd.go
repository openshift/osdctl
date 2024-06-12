package setup

import (
	"bufio"
	"fmt"
	"os"
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
)

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
				Dt_Vault_Path,
				Vault_Address,
			}

			values := make(map[string]string)
			reader := bufio.NewReader(os.Stdin)

			for _, key := range keys {
				fmt.Printf("Enter %s:", key)
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
