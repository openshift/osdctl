package managedpolicies

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

func NewCmdManagedPolicies() *cobra.Command {
	var cloudValue policies.CloudSpec
	var managedPoliciesCommand = &cobra.Command{
		Use:   "managedpolicies",
		Short: "STS/WIF utilities",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			if err != nil {
				fmt.Println("Error calling cmd.Help(): ", err.Error())
				return
			}
		},
	}
	managedPoliciesCommand.PersistentFlags().VarP(&cloudValue, "cloud", "c", "cloud for which the policies should be retrieved. supported values: [aws, sts, gcp, wif]")

	managedPoliciesCommand.AddCommand(newCmdGet())
	managedPoliciesCommand.AddCommand(newCmdDiff())
	managedPoliciesCommand.AddCommand(newCmdSave())

	return managedPoliciesCommand
}
