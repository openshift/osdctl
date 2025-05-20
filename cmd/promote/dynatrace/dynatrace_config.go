package dynatrace

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type DynatraceConfig struct {
	GitDirectory string
}

func DynatraceConfigPromotion(dynatraceConfigCheckoutDir string) DynatraceConfig {
	a := DynatraceConfig{}
	if dynatraceConfigCheckoutDir != "" {
		a.GitDirectory = dynatraceConfigCheckoutDir
		err := checkDynatraceConfigCheckout(a.GitDirectory)
		if err != nil {
			log.Fatalf("Provided directory %s is not an dynatrace-config directory: %v", a.GitDirectory, err)
		}
		return a
	}

	dir, err := getBaseDir()
	if err == nil {
		a.GitDirectory = dir
		err = checkDynatraceConfigCheckout(a.GitDirectory)
		if err == nil {
			return a
		}
	}

	log.Printf("Not running in Dynatrace Config directory: %v - Trying %s next\n", err, DefaultDynatraceconfigDir())
	a.GitDirectory = DefaultDynatraceconfigDir()
	err = checkDynatraceConfigCheckout(a.GitDirectory)
	if err != nil {
		log.Fatalf("%s is not an Dynatrace Config directory: %v", DefaultDynatraceconfigDir(), err)
	}

	log.Printf("Found Dynatrace Config in %s.\n", a.GitDirectory)
	return a
}

func DefaultDynatraceconfigDir() string {
	return filepath.Join(os.Getenv("HOME"), "git", "dynatrace-config")
}

func checkDynatraceConfigCheckout(directory string) error {
	cmd := exec.Command("git", "remote", "-v")
	cmd.Dir = directory
	output, err := cmd.CombinedOutput()
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
	cmd := exec.Command("git", "checkout", "main")
	cmd.Dir = a.GitDirectory
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to checkout master branch: %v", err)
	}

	cmd = exec.Command("git", "branch", "-D", branchName)
	cmd.Dir = a.GitDirectory
	err = cmd.Run()
	if err != nil {
		fmt.Printf("failed to cleanup branch %s: %v, continuing to create it.\n", branchName, err)
	}

	cmd = exec.Command("git", "checkout", "-b", branchName, "main")
	cmd.Dir = a.GitDirectory
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %v, does it already exist? If so, please delete it with `git branch -D %s` first", branchName, err, branchName)
	}

	return nil
}

func (a DynatraceConfig) commitFiles(commitMessage string) error {
	// Commit the change
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = a.GitDirectory
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to add file : %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", commitMessage)
	cmd.Dir = a.GitDirectory
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to commit changes: %v", err)
	}

	return nil
}
