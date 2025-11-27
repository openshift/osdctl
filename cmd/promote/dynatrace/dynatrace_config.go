package dynatrace

import (
	"fmt"
	"log"
	"os"

	"path/filepath"
	"strings"

	"github.com/openshift/osdctl/cmd/promote/git"
	"github.com/openshift/osdctl/cmd/promote/iexec"
)

type DynatraceConfig struct {
	GitDirectory string
	GitExecutor  iexec.IExec
}

func DynatraceConfigPromotion(dynatraceConfigCheckoutDir string) DynatraceConfig {
	a := DynatraceConfig{}
	a.GitExecutor = iexec.Exec{}
	if dynatraceConfigCheckoutDir != "" {
		a.GitDirectory = dynatraceConfigCheckoutDir
		err := a.checkDynatraceConfigCheckout()
		if err != nil {
			log.Fatalf("Provided directory %s is not an dynatrace-config directory: %v", a.GitDirectory, err)
		}
		return a
	}

	dir, err := git.GetBaseDir(a.GitExecutor)
	if err == nil {
		a.GitDirectory = dir
		err = a.checkDynatraceConfigCheckout()
		if err == nil {
			return a
		}
	}

	log.Printf("Not running in Dynatrace Config directory: %v - Trying %s next\n", err, DefaultDynatraceconfigDir())
	a.GitDirectory = DefaultDynatraceconfigDir()
	err = a.checkDynatraceConfigCheckout()
	if err != nil {
		log.Fatalf("%s is not an Dynatrace Config directory: %v", DefaultDynatraceconfigDir(), err)
	}

	log.Printf("Found Dynatrace Config in %s.\n", a.GitDirectory)
	return a
}

func DefaultDynatraceconfigDir() string {
	return filepath.Join(os.Getenv("HOME"), "git", "dynatrace-config")
}

func (a DynatraceConfig) checkDynatraceConfigCheckout() error {
	output, err := a.GitExecutor.CombinedOutput(a.GitDirectory, "git", "remote", "-v")
	if err != nil {
		return fmt.Errorf("error executing 'git remote -v': %v", err)
	}

	outputString := string(output)

	// Check if the output contains the dynatrace-config repository URL
	if !strings.Contains(outputString, "gitlab.cee.redhat.com") && !strings.Contains(outputString, "dynatrace-config") {
		return fmt.Errorf("not running in checkout of dynatrace-config")
	}
	//fmt.Println("Running in checkout of dynatrace-config.")

	return nil
}

func (a DynatraceConfig) UpdateDynatraceConfig(component, promotionGitHash, branchName string) error {
	err := a.GitExecutor.Run(a.GitDirectory, "git", "checkout", "main")
	if err != nil {
		return fmt.Errorf("failed to checkout master branch: %v", err)
	}

	err = a.GitExecutor.Run(a.GitDirectory, "git", "branch", "-D", branchName)
	if err != nil {
		fmt.Printf("failed to cleanup branch %s: %v, continuing to create it.\n", branchName, err)
	}

	err = a.GitExecutor.Run(a.GitDirectory, "git", "checkout", "-b", branchName, "main")
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %v, does it already exist? If so, please delete it with `git branch -D %s` first", branchName, err, branchName)
	}

	return nil
}

func (a DynatraceConfig) commitFiles(commitMessage string) error {
	// Commit the change
	err := a.GitExecutor.Run(a.GitDirectory, "git", "add", ".")
	if err != nil {
		return fmt.Errorf("failed to add file : %v", err)
	}

	err = a.GitExecutor.Run(a.GitDirectory, "git", "commit", "-m", commitMessage)
	if err != nil {
		return fmt.Errorf("failed to commit changes: %v", err)
	}

	return nil
}
