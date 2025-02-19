package dynatrace

import (
	"fmt"
	"os"

	git "github.com/openshift/osdctl/cmd/promote/git"
	"github.com/spf13/cobra"
)

type promoteDynatraceOptions struct {
	list bool

	appInterfaceCheckoutDir string
	gitHash                 string
	component               string
}

// NewCmdPromote implements the promote command to promote services/operators
func NewCmdDynatrace() *cobra.Command {
	ops := &promoteDynatraceOptions{}
	promoteDynatraceCmd := &cobra.Command{
		Use:               "dynatrace",
		Short:             "Utilities to promote dynatrace",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Example: `
		# List all Dynatrace components available for promotion
		osdctl promote dynatrace --list

		# Promote a dynatrace component
		osdctl promote dynatrace --component <component> --gitHash <git-hash>`,

		Run: func(cmd *cobra.Command, args []string) {
			ops.validateSaasFlow()
			appInterface := BootstrapOsdCtlForAppInterfaceAndServicePromotions(ops.appInterfaceCheckoutDir)

			if ops.list {
				if ops.component != "" || ops.gitHash != "" {
					fmt.Printf("Error: --list cannot be used with any other flags\n\n")
					cmd.Help()
					os.Exit(1)
				}
				listServiceNames(appInterface)
				os.Exit(0)
			}

			if ops.component == "" {
				fmt.Println("Error: Please provide dynatrace component to promote.\n\n")
				fmt.Println("Please run 'osdctl promote dynatrace --list' to check available dynatrace components for promotion.\n\n")
				cmd.Help()
				os.Exit(1)
			}
			err := servicePromotion(appInterface, ops.component, ops.gitHash)
			if err != nil {
				fmt.Printf("Error while promoting service: %v\n", err)
				os.Exit(1)
			}

			os.Exit(0)
		},
	}

	promoteDynatraceCmd.Flags().BoolVarP(&ops.list, "list", "l", false, "List all SaaS services/operators")
	promoteDynatraceCmd.Flags().StringVarP(&ops.component, "component", "c", "", "Dynatrace component getting promoted")
	promoteDynatraceCmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "Git hash of the SaaS service/operator commit getting promoted")
	promoteDynatraceCmd.Flags().StringVarP(&ops.appInterfaceCheckoutDir, "appInterfaceDir", "", "", "location of app-interfache checkout. Falls back to `pwd` and "+git.DefaultAppInterfaceDirectory())

	return promoteDynatraceCmd
}

func (o *promoteDynatraceOptions) validateSaasFlow() {
	if o.component == "" && o.gitHash == "" {
		fmt.Printf("Usage: For dynatrace component, please provide --component and (optional) --gitHash\n")
		fmt.Printf("--component is the name of the component, i.e. dynatrace-dynakube\n")
		fmt.Printf("--gitHash is the target git commit for the dt component, if not specified defaults to HEAD of master\n\n")
		return
	}
}
