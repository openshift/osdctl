package dynatrace

import (
	"fmt"
	"os"

	"github.com/openshift/osdctl/cmd/promote/git"
	"github.com/openshift/osdctl/cmd/promote/iexec"
	"github.com/spf13/cobra"
)

type promoteDynatraceOptions struct {
	list bool

	appInterfaceCheckoutDir    string
	gitHash                    string
	component                  string
	terraform                  bool
	module                     string
	dynatraceConfigCheckoutDir string
}

// NewCmdDynatrace implements the promote command to promote services/operators
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
		osdctl promote dynatrace --component <component> --gitHash <git-hash>

		# List all dynatrace-config modules available for promotion
		osdctl promote dynatrace --terraform --list

		# Promote a dynatrace module
		osdctl promote dynatrace --terraform --module=<module-name>`,

		Run: func(cmd *cobra.Command, args []string) {

			if ops.terraform {
				dynatraceConfig := DynatraceConfigPromotion(ops.dynatraceConfigCheckoutDir)
				if ops.list {
					ops.validateSaasFlow()
					if ops.component != "" || ops.gitHash != "" || ops.module != "" {
						fmt.Printf("Error: Please provide correct parameters \n\n")
						fmt.Printf("Please run 'osdctl promote dynatrace --terraform --list' to check available dynatrace-config modules for promotion.\n\n")
						cmd.Help()
						os.Exit(1)
					}
					_ = listDynatraceModuleNames(dynatraceConfig)
					os.Exit(0)
				} else {
					if ops.module == "" {
						fmt.Printf("Error: Please provide correct parameters \n\n")
						fmt.Printf("Please run 'osdctl promote dynatrace --terraform --module=<module-name>' to check promote dynatrace-config module to latest ref.\n\n")
						cmd.Help()
						os.Exit(1)
					} else if ops.component != "" || ops.gitHash != "" {
						fmt.Printf("Error: Please provide correct parameters \n\n")
						fmt.Printf("Please run 'osdctl promote dynatrace --terraform --module=<module-name>' to check promote dynatrace-config module to latest ref.\n\n")
						cmd.Help()
						os.Exit(1)
					} else {
						err := modulePromotion(dynatraceConfig, ops.module)
						if err != nil {
							fmt.Printf("Error while promoting module: %v\n", err)
							os.Exit(1)
						}
					}
				}
			} else {

				ops.validateSaasFlow()
				appInterface := git.BootstrapOsdCtlForAppInterfaceAndServicePromotions(ops.appInterfaceCheckoutDir, iexec.Exec{})
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
					fmt.Printf("Error: Please provide dynatrace component to promote.\n\n")
					fmt.Printf("Please run 'osdctl promote dynatrace --list' to check available dynatrace components for promotion.\n\n")
					cmd.Help()
					os.Exit(1)
				}
				err := servicePromotion(appInterface, ops.component, ops.gitHash)
				if err != nil {
					fmt.Printf("Error while promoting service: %v\n", err)
					os.Exit(1)
				}
			}
			os.Exit(0)
		},
	}

	promoteDynatraceCmd.Flags().BoolVarP(&ops.list, "list", "l", false, "List all SaaS services/operators")
	promoteDynatraceCmd.Flags().StringVarP(&ops.component, "component", "c", "", "Dynatrace component getting promoted")
	promoteDynatraceCmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "Git hash of the SaaS service/operator commit getting promoted")
	promoteDynatraceCmd.Flags().StringVarP(&ops.appInterfaceCheckoutDir, "appInterfaceDir", "", "", "location of app-interface checkout. Falls back to current working directory")
	promoteDynatraceCmd.Flags().BoolVarP(&ops.terraform, "terraform", "t", false, "deploy dynatrace-config terraform job")
	promoteDynatraceCmd.Flags().StringVarP(&ops.module, "module", "m", "", "module to promote")
	promoteDynatraceCmd.Flags().StringVarP(&ops.dynatraceConfigCheckoutDir, "dynatraceConfigDir", "", "", "location of dynatrace-config checkout. Falls back to current working directory")

	return promoteDynatraceCmd
}

func (o *promoteDynatraceOptions) validateSaasFlow() {
	if o.component == "" && o.gitHash == "" && !o.terraform {
		fmt.Printf("Usage: For dynatrace component, please provide --component and (optional) --gitHash\n")
		fmt.Printf("--component is the name of the component, i.e. dynatrace-dynakube\n")
		fmt.Printf("--gitHash is the target git commit for the dt component, if not specified defaults to HEAD of master\n\n")
		return
	}
}
