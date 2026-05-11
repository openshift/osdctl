package blocked

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/openshift/osdctl/pkg/promote"
	"github.com/spf13/cobra"
)

type blockedOptions struct {
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
		# Block a specific version for a component
		osdctl promote blocked --serviceId <service> --component <component-name> --gitHash <sha>

		# With explicit app-interface path
		osdctl promote blocked --serviceId <service> --component <component-name> --gitHash <sha> --appInterfaceDir /path/to/app-interface`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if ops.serviceId == "" {
				return fmt.Errorf("--serviceId is required")
			}
			if ops.componentName == "" {
				return fmt.Errorf("--component is required")
			}
			if ops.gitHash == "" {
				return fmt.Errorf("--gitHash is required")
			}

			cmd.SilenceUsage = true

			appInterfaceClone, err := promote.FindAppInterfaceClone(ops.appInterfaceProvidedPath)
			if err != nil {
				return err
			}

			isClean, err := appInterfaceClone.IsClean()
			if err != nil {
				return err
			}
			if !isClean {
				return fmt.Errorf("app-interface clone in '%s' has uncommitted changes, please commit or stash them before proceeding", appInterfaceClone.GetPath())
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

			service, err := servicesRegistry.GetService(ops.serviceId)
			if err != nil {
				return err
			}

			application := service.GetApplication()

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

	blockedCmd.Flags().StringVarP(&ops.serviceId, "serviceId", "", "", "Name of the SaaS service file (without extension)")
	blockedCmd.Flags().StringVarP(&ops.componentName, "component", "c", "", "Name of the code component in app.yaml")
	blockedCmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "SHA commit hash to add to blockedVersions")
	blockedCmd.Flags().StringVarP(&ops.appInterfaceProvidedPath, "appInterfaceDir", "", "", "Location of app-interface checkout. Falls back to the current working directory")

	return blockedCmd
}
