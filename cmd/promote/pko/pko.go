package pko

import (
	"fmt"

	"github.com/openshift/osdctl/cmd/promote/git"
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
			cmdutil.CheckErr(ops.ValidatePKOOptions())
			git.BootstrapOsdCtlForAppInterfaceAndServicePromotions()

			cmdutil.CheckErr(PromotePackage(ops.serviceName, ops.packageTag, ops.hcp))
		},
	}
	pkoCmd.Flags().StringVarP(&ops.serviceName, "serviceName", "n", "", "Service getting promoted")
	pkoCmd.Flags().StringVarP(&ops.packageTag, "tag", "t", "", "Package tag being promoted to")
	pkoCmd.Flags().BoolVar(&ops.hcp, "hcp", false, "The service being promoted conforms to the HyperShift progressive delivery definition")
	return pkoCmd
}

// pkoOptions defines the options provided by this command
type pkoOptions struct {
	serviceName string
	packageTag  string
	hcp         bool
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

func PromotePackage(serviceName string, packageTag string, hcp bool) error {
	services, err := saas.GetServiceNames(saas.OSDSaasDir, saas.BPSaasDir, saas.CADSaasDir)
	if err != nil {
		return err
	}

	err = saas.ValidateServiceName(services, serviceName)
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
	err = git.UpdatePackageTag(saasFile, currentTag, packageTag, branchName)
	if err != nil {
		return err
	}

	commitMessage := fmt.Sprintf("Promote %s package to %s", serviceName, packageTag)
	err = git.CommitSaasFile(saasFile, commitMessage)
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
