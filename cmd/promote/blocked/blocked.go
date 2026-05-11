package blocked

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/openshift/osdctl/pkg/promote"
	"github.com/spf13/cobra"
)

type blockedOptions struct {
	list bool
	all  bool

	appInterfaceProvidedPath string
	serviceId                string
	componentName            string
	gitHash                  string
}

// NewCmdBlock implements the block command to add a blocked version to a component in app.yaml
func NewCmdBlock() *cobra.Command {
	ops := &blockedOptions{}
	blockedCmd := &cobra.Command{
		Use:   "block",
		Short: "Add a blocked version to a component in app.yaml",
		Long: `Add a SHA commit hash to the blockedVersions list for a code component
in the application's app.yaml file. This prevents the specified version
from being promoted through progressive delivery.

The command locates the app.yaml through the SaaS service file, finds
the specified component by name, and appends the git hash to its
codeComponents[].blockedVersions array. If the blockedVersions field
does not yet exist, it will be created.

Duplicate entries are rejected with an error.`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Example: `
		# List all services and their components
		osdctl promote block --list

		# Block a specific version for a single component
		osdctl promote block --serviceId <service> --component <component-name> --gitHash <sha>

		# Block a specific version for all components of a service
		osdctl promote block --serviceId <service> --all --gitHash <sha>

		# With explicit app-interface path
		osdctl promote block --serviceId <service> --component <component-name> --gitHash <sha> --appInterfaceDir /path/to/app-interface`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if ops.list {
				if ops.serviceId != "" || ops.componentName != "" || ops.gitHash != "" || ops.all {
					return fmt.Errorf("--list cannot be used with --serviceId, --component, --all or --gitHash")
				}
			} else {
				if ops.serviceId == "" {
					return fmt.Errorf("--serviceId is required (use --list to see available services and components)")
				}
				if ops.all && ops.componentName != "" {
					return fmt.Errorf("--all and --component are mutually exclusive")
				}
				if !ops.all && ops.componentName == "" {
					return fmt.Errorf("--component or --all is required (use --list to see available services and components)")
				}
				if ops.gitHash == "" {
					return fmt.Errorf("--gitHash is required")
				}
			}

			cmd.SilenceUsage = true

			appInterfaceClone, err := promote.FindAppInterfaceClone(ops.appInterfaceProvidedPath)
			if err != nil {
				return err
			}

			servicesRegistry, err := promote.NewServicesRegistry(
				appInterfaceClone,
				func(filePath string) string { return filePath },
				"data/services/osd-operators/cicd/saas",
				"data/services/backplane/cicd/saas",
				"data/services/configuration-anomaly-detection/cicd",
			)
			if err != nil {
				return err
			}

			if ops.list {
				fmt.Println("### Services and their components ###")
				for _, serviceId := range servicesRegistry.GetServicesIds() {
					service, err := servicesRegistry.GetService(serviceId)
					if err != nil {
						fmt.Printf("  %s (error: %v)\n", serviceId, err)
						continue
					}
					componentNames, err := service.GetApplication().GetComponentNames()
					if err != nil {
						fmt.Printf("  %s (error reading components: %v)\n", serviceId, err)
						continue
					}
					fmt.Printf("  %s\n", serviceId)
					for _, name := range componentNames {
						fmt.Printf("    - %s\n", name)
					}
				}
				return nil
			}

			service, err := servicesRegistry.GetService(ops.serviceId)
			if err != nil {
				return err
			}

			application := service.GetApplication()

			isClean, err := appInterfaceClone.IsClean()
			if err != nil {
				return err
			}
			if !isClean {
				return fmt.Errorf("app-interface clone in '%s' has uncommitted changes, please commit or stash them before proceeding", appInterfaceClone.GetPath())
			}

			branchName := fmt.Sprintf("block-%s-%s", ops.serviceId, ops.gitHash)
			err = appInterfaceClone.CheckoutNewBranch(branchName)
			if err != nil {
				return err
			}

			var components []*promote.CodeComponent

			if ops.all {
				components, err = application.GetAllComponents()
				if err != nil {
					return err
				}
			} else {
				component, err := application.GetComponentByName(ops.componentName)
				if err != nil {
					return err
				}
				components = []*promote.CodeComponent{component}
			}

			var blockedNames []string
			for _, component := range components {
				err = component.AddBlockedVersion(ops.gitHash)
				if err != nil {
					return err
				}
				blockedNames = append(blockedNames, component.GetName())
			}

			err = application.Save()
			if err != nil {
				return fmt.Errorf("failed to save application '%s': %v", application.GetFilePath(), err)
			}

			targetLabel := strings.Join(blockedNames, ", ")

			var commitMessage string
			if ops.all {
				commitMessage = fmt.Sprintf("Block version %s for all components of %s\n\nAdd %s to blockedVersions for components [%s] in '%s'.",
					ops.gitHash,
					ops.serviceId,
					ops.gitHash,
					targetLabel,
					filepath.Base(application.GetFilePath()),
				)
			} else {
				commitMessage = fmt.Sprintf("Block version %s for %s\n\nAdd %s to blockedVersions for component '%s' in '%s'.",
					ops.gitHash,
					ops.componentName,
					ops.gitHash,
					ops.componentName,
					filepath.Base(application.GetFilePath()),
				)
			}

			err = appInterfaceClone.Commit(commitMessage)
			if err != nil {
				return err
			}

			fmt.Println("SUCCESS!")
			fmt.Printf("Blocked version %s for: %s\n", ops.gitHash, targetLabel)
			fmt.Printf("Application file: %s\n", application.GetFilePath())
			fmt.Println("")
			fmt.Println("-------------    Commit message     -------------")
			fmt.Println(commitMessage)
			fmt.Println("------------- End of commit message -------------")
			fmt.Println("")
			fmt.Printf("Push the following branch on your fork and create a MR from it: %s\n", branchName)

			appInterfacePath := appInterfaceClone.GetPath()
			if strings.Contains(appInterfacePath, "app-interface") {
				fmt.Printf("\n(reminder: the push has to be run from the following Git clone: %s)\n", appInterfacePath)
			}

			return nil
		},
	}

	blockedCmd.Flags().BoolVarP(&ops.list, "list", "l", false, "List all services and their components")
	blockedCmd.Flags().BoolVarP(&ops.all, "all", "a", false, "Block the version for all components of the service (mutually exclusive with --component)")
	blockedCmd.Flags().StringVarP(&ops.serviceId, "serviceId", "", "", "Name of the SaaS service file (without extension)")
	blockedCmd.Flags().StringVarP(&ops.componentName, "component", "c", "", "Name of the code component in app.yaml")
	blockedCmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "SHA commit hash to add to blockedVersions")
	blockedCmd.Flags().StringVarP(&ops.appInterfaceProvidedPath, "appInterfaceDir", "", "", "Location of app-interface checkout. Falls back to the current working directory")
	blockedCmd.MarkFlagsMutuallyExclusive("all", "component")

	return blockedCmd
}
