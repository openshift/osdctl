package pko

import (
	"fmt"

	"github.com/openshift/osdctl/cmd/promote/git"
	"github.com/openshift/osdctl/cmd/promote/iexec"
	"github.com/openshift/osdctl/cmd/promote/saas"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewCmdPKO() *cobra.Command {
	ops := &pkoOptions{}

	pkoCmd := &cobra.Command{
		Use:               "package",
		Short:             "Utilities to promote package-operator services",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Example: `
 # Promote a package-operator service
 osdctl promote package --serviceName <serviceName> --gitHash <git-hash>`,
		Run: func(cmd *cobra.Command, args []string) {
			// Set default directory if not provided
			if ops.appInterfaceCheckoutDir == "" {
				ops.appInterfaceCheckoutDir = git.DefaultAppInterfaceDirectory()
			}

			cmdutil.CheckErr(ops.ValidatePKOOptions())
			appInterface := git.BootstrapOsdCtlForAppInterfaceAndServicePromotions(ops.appInterfaceCheckoutDir, iexec.Exec{})
			cmdutil.CheckErr(PromotePackage(appInterface, ops.serviceName, ops.packageTag, ops.hcp))
		},
	}

	pkoCmd.Flags().StringVarP(&ops.serviceName, "serviceName", "n", "", "Service getting promoted")
	pkoCmd.Flags().StringVarP(&ops.packageTag, "tag", "t", "", "Package tag being promoted to")
	pkoCmd.Flags().StringVarP(&ops.appInterfaceCheckoutDir, "appInterfaceDir", "", "", "location of app-interface checkout. Falls back to current working directory")
	pkoCmd.Flags().BoolVar(&ops.hcp, "hcp", false, "The service being promoted conforms to the HyperShift progressive delivery definition")

	return pkoCmd
}

// pkoOptions defines the options provided by this command
type pkoOptions struct {
	serviceName             string
	packageTag              string
	appInterfaceCheckoutDir string
	hcp                     bool
}

func (p pkoOptions) ValidatePKOOptions() error {
	if p.serviceName == "" {
		return fmt.Errorf("the service name must be specified with --serviceName/-s")
	}
	if p.packageTag == "" {
		return fmt.Errorf("a new package tag must be provided with '--tag' or '-t'")
	}
	return nil
}

func PromotePackage(appInterface git.AppInterface, serviceName string, packageTag string, hcp bool) error {
	// Fetch and sync with origin/master FIRST before reading any files
	// to ensure we're working with the latest app-interface state
	fmt.Println("Syncing app-interface with origin/master...")
	if err := appInterface.GitExecutor.Run(appInterface.GitDirectory, "git", "fetch", "origin", "master"); err != nil {
		return fmt.Errorf("failed to fetch origin/master: %v", err)
	}
	if err := appInterface.GitExecutor.Run(appInterface.GitDirectory, "git", "checkout", "master"); err != nil {
		return fmt.Errorf("failed to checkout master: %v", err)
	}
	if err := appInterface.GitExecutor.Run(appInterface.GitDirectory, "git", "reset", "--hard", "origin/master"); err != nil {
		return fmt.Errorf("failed to sync master with origin/master: %v", err)
	}
	fmt.Println("Successfully synced local master with origin/master")

	services, err := saas.GetServiceNames(appInterface, saas.OSDSaasDir, saas.BPSaasDir, saas.CADSaasDir)
	if err != nil {
		return err
	}
	serviceName, err = saas.ValidateServiceName(services, serviceName)
	if err != nil {
		return err
	}
	saasFile, err := saas.GetSaasDir(serviceName, !hcp, hcp)
	if err != nil {
		return err
	}
	currentTag, err := git.GetCurrentPackageTagFromAppInterface(saasFile)
	if err != nil {
		return err
	}
	if currentTag == packageTag {
		return fmt.Errorf("current hash is already at '%s'. Nothing to do", packageTag)
	}
	branchName := fmt.Sprintf("promote-%s-package-%s", serviceName, packageTag)
	err = appInterface.UpdatePackageTag(saasFile, currentTag, packageTag, branchName)
	if err != nil {
		return err
	}
	commitMessage := fmt.Sprintf("Promote %s package to %s", serviceName, packageTag)
	err = appInterface.CommitSaasFile(saasFile, commitMessage)
	if err != nil {
		return err
	}

	fmt.Printf("The current branch (%s) is ready to be pushed\n", branchName)
	fmt.Println("")
	fmt.Printf("Service: %s\n", serviceName)
	fmt.Printf("Previous Tag: %s\n", currentTag)
	fmt.Printf("New Tag: %s\n", packageTag)
	return nil
}

func updatePackageHash(gitHash, saasFile string) error {
	return nil
}
