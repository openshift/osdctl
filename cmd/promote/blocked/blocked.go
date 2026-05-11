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

	appInterfaceProvidedPath string
	serviceId                string
	componentName            string
	gitHash                  string
}

// NewCmdBlocked implements the blocked command to add a blocked version to a component in app.yaml
func NewCmdBlocked() *cobra.Command {
	ops := &blockedOptions{}
	blockedCmd := &cobra.Command{
		Use:   "blocked",
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
		osdctl promote blocked --list

		# Block a specific version for a component
		osdctl promote blocked --serviceId <service> --component <component-name> --gitHash <sha>

		# With explicit app-interface path
		osdctl promote blocked --serviceId <service> --component <component-name> --gitHash <sha> --appInterfaceDir /path/to/app-interface`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if ops.list {
				if ops.serviceId != "" || ops.componentName != "" || ops.gitHash != "" {
					return fmt.Errorf("--list cannot be used with --serviceId, --component or --gitHash")
				}
			} else {
				if ops.serviceId == "" {
					return fmt.Errorf("--serviceId is required (use --list to see available services and components)")
				}
				if ops.componentName == "" {
					return fmt.Errorf("--component is required (use --list to see available services and components)")
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

			component, err := application.GetComponentByName(ops.componentName)
			if err != nil {
				return err
			}

			err = component.AddBlockedVersion(ops.gitHash)
			if err != nil {
				return err
			}

			err = application.Save()
			if err != nil {
				return fmt.Errorf("failed to save application '%s': %v", application.GetFilePath(), err)
			}

			branchName := fmt.Sprintf("block-%s-%s", ops.componentName, ops.gitHash)
			err = appInterfaceClone.CheckoutNewBranch(branchName)
			if err != nil {
				return err
			}

			commitMessage := fmt.Sprintf("Block version %s for %s\n\nAdd %s to blockedVersions for component '%s' in '%s'.",
				ops.gitHash,
				ops.componentName,
				ops.gitHash,
				ops.componentName,
				filepath.Base(application.GetFilePath()),
			)

			err = appInterfaceClone.Commit(commitMessage)
			if err != nil {
				return err
			}

			fmt.Println("SUCCESS!")
			fmt.Printf("Blocked version %s for component %s\n", ops.gitHash, ops.componentName)
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
	blockedCmd.Flags().StringVarP(&ops.serviceId, "serviceId", "", "", "Name of the SaaS service file (without extension)")
	blockedCmd.Flags().StringVarP(&ops.componentName, "component", "c", "", "Name of the code component in app.yaml")
	blockedCmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "SHA commit hash to add to blockedVersions")
	blockedCmd.Flags().StringVarP(&ops.appInterfaceProvidedPath, "appInterfaceDir", "", "", "Location of app-interface checkout. Falls back to the current working directory")

	return blockedCmd
}
