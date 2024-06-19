package iampermissions

import (
	"fmt"

	"github.com/openshift/osdctl/pkg/policies"
	"github.com/spf13/cobra"
)

const (
	// cloudFlagName is declared as const so we can keep naming of this flag
	// consistent between different subcommands
	cloudFlagName = "cloud"
)

func NewCmdIamPermissions() *cobra.Command {
	var cloudValue policies.CloudSpec
	var iamPermissionsCommand = &cobra.Command{
		Use:   "iampermissions",
		Short: "STS/WIF utilities",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			if err != nil {
				fmt.Println("Error calling cmd.Help(): ", err.Error())
				return
			}
		},
	}
	iamPermissionsCommand.PersistentFlags().VarP(&cloudValue, "cloud", "c", "cloud for which the policies should be retrieved. supported values: [aws, sts, gcp, wif]")

	iamPermissionsCommand.AddCommand(newCmdGet())
	iamPermissionsCommand.AddCommand(newCmdDiff())
	iamPermissionsCommand.AddCommand(newCmdSave())

	return iamPermissionsCommand
}
