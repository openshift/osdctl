package dynatrace

import (
	"errors"
	"fmt"

	"github.com/openshift/osdctl/pkg/promote"
	"github.com/spf13/cobra"
)

type promoteDynatraceOptions struct {
	list bool

	appInterfaceProvidedPath   string
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
		Long: `Promote Dynatrace components or terraform modules.

DYNATRACE COMPONENTS:
  Components are defined in app-interface under:
    data/services/osd-operators/cicd/saas/saas-dynatrace/

  Each component maps to a specific path within the dynatrace-config repository.
  When promoting a component, only changes to that component's path are included
  in the promotion diff.

  Please run 'osdctl promote dynatrace --list' to check available dynatrace components for promotion & their corresponding paths.

TERRAFORM MODULES:
  Modules are defined in the dynatrace-config repository under:
    terraform/modules/

  Promoting a module updates configs in:
    terraform/redhat-aws/sd-sre/`,
		Example: `
		# List all Dynatrace components available for promotion
		osdctl promote dynatrace --list

		# Promote a dynatrace component
		osdctl promote dynatrace --component <component> --gitHash <git-hash>

		# List all dynatrace-config modules available for promotion
		osdctl promote dynatrace --terraform --list

		# Promote a dynatrace module
		osdctl promote dynatrace --terraform --module=<module-name>`,

		RunE: func(cmd *cobra.Command, args []string) error {

			if ops.terraform {
				dynatraceConfig := DynatraceConfigPromotion(ops.dynatraceConfigCheckoutDir)
				if ops.list {
					ops.validateSaasFlow()
					if ops.component != "" || ops.gitHash != "" || ops.module != "" {
						return errors.New("--list cannot be used with --component, --gitHash or --module")
					}

					cmd.SilenceUsage = true

					return listDynatraceModuleNames(dynatraceConfig)
				} else {
					if ops.module == "" {
						return errors.New("--module is required unless --list is used")
					}
					if ops.component != "" || ops.gitHash != "" {
						return errors.New("--component and --gitHash cannot be used with --terraform")
					}

					cmd.SilenceUsage = true

					err := modulePromotion(dynatraceConfig, ops.module)
					if err != nil {
						return fmt.Errorf("error while promoting module: %v", err)
					}
				}
			} else {
				ops.validateSaasFlow()

				appInterfaceClone, err := promote.FindAppInterfaceClone(ops.appInterfaceProvidedPath)
				if err != nil {
					return err
				}

				servicesRegistry, err := promote.NewServicesRegistry(
					appInterfaceClone,
					validateDynatraceServiceFilePath,
					saasDynatraceDir,
				)
				if err != nil {
					return err
				}

				if ops.list {
					if ops.component != "" || ops.gitHash != "" {
						return errors.New("--list cannot be used with --component or --gitHash")
					}

					cmd.SilenceUsage = true

					return listServiceIds(servicesRegistry)
				} else {
					if ops.component == "" {
						return errors.New("--component is required unless --list is used")
					}

					cmd.SilenceUsage = true

					service, err := servicesRegistry.GetService(ops.component)
					if err != nil {
						return err
					}
					err = service.Promote(&promote.DefaultPromoteCallbacks{Service: service}, ops.gitHash)

					if err != nil {
						return fmt.Errorf("error while promoting service: %v", err)
					}
				}
			}
			return nil
		},
	}

	promoteDynatraceCmd.Flags().BoolVarP(&ops.list, "list", "l", false, "List all SaaS services/operators")
	promoteDynatraceCmd.Flags().StringVarP(&ops.component, "component", "c", "", "Dynatrace component getting promoted (ex: dynatrace-dynakube)")
	promoteDynatraceCmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "Git hash of the component getting promoted from dynatrace-config repo")
	promoteDynatraceCmd.Flags().StringVarP(&ops.appInterfaceProvidedPath, "appInterfaceDir", "", "", "Location of app-interface checkout. Falls back to current working directory")
	promoteDynatraceCmd.Flags().BoolVarP(&ops.terraform, "terraform", "t", false, "Deploy dynatrace-config terraform job")
	promoteDynatraceCmd.Flags().StringVarP(&ops.module, "module", "m", "", "Module to promote")
	promoteDynatraceCmd.Flags().StringVarP(&ops.dynatraceConfigCheckoutDir, "dynatraceConfigDir", "", "", "Location of dynatrace-config checkout. Falls back to current working directory")

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
