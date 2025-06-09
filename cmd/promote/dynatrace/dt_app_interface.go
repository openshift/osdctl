package dynatrace

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift/osdctl/cmd/promote/iexec"
	"gopkg.in/yaml.v3"
)

type Service struct {
	Name              string `yaml:"name"`
	ResourceTemplates []struct {
		Name    string `yaml:"name"`
		URL     string `yaml:"url"`
		PATH    string `yaml:"path"`
		Targets []struct {
			Name       string
			Namespace  map[string]string      `yaml:"namespace"`
			Ref        string                 `yaml:"ref"`
			Parameters map[string]interface{} `yaml:"parameters"`
		} `yaml:"targets"`
	} `yaml:"resourceTemplates"`
}

type AppInterface struct {
	GitDirectory string
	GitExecutor  iexec.IExec
}

func DefaultAppInterfaceDirectory() string {
	return filepath.Join(os.Getenv("HOME"), "git", "app-interface")
}

func BootstrapOsdCtlForAppInterfaceAndServicePromotions(appInterfaceCheckoutDir string) AppInterface {
	a := AppInterface{}
	a.GitExecutor = iexec.Exec{}
	if appInterfaceCheckoutDir != "" {
		a.GitDirectory = appInterfaceCheckoutDir
		err := a.checkAppInterfaceCheckout()
		if err != nil {
			log.Fatalf("Provided directory %s is not an AppInterface directory: %v", a.GitDirectory, err)
		}
		return a
	}

	dir, err := getBaseDir(a.GitExecutor)
	if err == nil {
		a.GitDirectory = dir
		err = a.checkAppInterfaceCheckout()
		if err == nil {
			return a
		}
	}

	log.Printf("Not running in AppInterface directory: %v - Trying %s next\n", err, DefaultAppInterfaceDirectory())
	a.GitDirectory = DefaultAppInterfaceDirectory()
	err = a.checkAppInterfaceCheckout()
	if err != nil {
		log.Fatalf("%s is not an AppInterface directory: %v", DefaultAppInterfaceDirectory(), err)
	}

	log.Printf("Found AppInterface in %s.\n", a.GitDirectory)
	return a
}

// checkAppInterfaceCheckout checks if the script is running in the checkout of app-interface
func (a AppInterface) checkAppInterfaceCheckout() error {
	output, err := a.GitExecutor.Output(a.GitDirectory, "git", "remote", "-v")
	if err != nil {
		return fmt.Errorf("error executing 'git remote -v': %v", err)
	}

	outputString := string(output)

	// Check if the output contains the app-interface repository URL
	if !strings.Contains(outputString, "gitlab.cee.redhat.com") && !strings.Contains(outputString, "app-interface") {
		return fmt.Errorf("not running in checkout of app-interface")
	}
	//fmt.Println("Running in checkout of app-interface.")

	return nil
}

func GetCurrentGitHashFromAppInterface(saarYamlFile []byte, serviceName string) (string, string, string, error) {
	var currentGitHash string
	var serviceRepo string
	var service Service
	var serviceFilepath string

	err := yaml.Unmarshal(saarYamlFile, &service)
	if err != nil {
		log.Fatal(fmt.Errorf("cannot unmarshal yaml data of service %s: %v", serviceName, err))
	}

	for _, resourceTemplate := range service.ResourceTemplates {
		for _, target := range resourceTemplate.Targets {
			if strings.Contains(target.Name, "production-") || strings.Contains(target.Namespace["$ref"], "hivep") {
				currentGitHash = target.Ref
				serviceFilepath = resourceTemplate.PATH
				break
			}
		}
	}

	if currentGitHash == "" {
		return "", "", "", fmt.Errorf("production namespace not found for service %s", serviceName)
	}

	if len(service.ResourceTemplates) > 0 {
		serviceRepo = service.ResourceTemplates[0].URL
	}

	if serviceRepo == "" {
		return "", "", "", fmt.Errorf("service repo not found for service %s", serviceName)
	}

	return currentGitHash, serviceRepo, serviceFilepath, nil
}

func (a AppInterface) UpdateAppInterface(component, saasFile, currentGitHash, promotionGitHash, branchName string) error {
	if err := a.GitExecutor.Run(a.GitDirectory, "git", "checkout", "master"); err != nil {
		return fmt.Errorf("failed to checkout master: branch %v", err)
	}

	if err := a.GitExecutor.Run(a.GitDirectory, "git", "branch", "-D", branchName); err != nil {
		fmt.Printf("failed to cleanup branch %s: %v, continuing to create it.\n", branchName, err)
	}

	if err := a.GitExecutor.Run(a.GitDirectory, "git", "checkout", "-b", branchName, "master"); err != nil {
		return fmt.Errorf("failed to create branch %s: %v, does it already exist? If so, please delete it with `git branch -D %s` first", branchName, err, branchName)
	}

	// Update the hash in the SAAS file
	fileContent, err := os.ReadFile(saasFile)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", saasFile, err)
	}

	// Replace the hash in the file content
	newContent := strings.ReplaceAll(string(fileContent), currentGitHash, promotionGitHash)

	err = os.WriteFile(saasFile, []byte(newContent), 0600)
	if err != nil {
		return fmt.Errorf("failed to write to file %s: %v", saasFile, err)
	}

	return nil
}

func (a AppInterface) UpdatePackageTag(saasFile, oldTag, promotionTag, branchName string) error {
	if err := a.GitExecutor.Run(a.GitDirectory, "git", "checkout", "master"); err != nil {
		return fmt.Errorf("failed to checkout master branch: %v", err)
	}

	if err := a.GitExecutor.Run(a.GitDirectory, "git", "branch", "-D", branchName); err != nil {
		fmt.Printf("failed to cleanup branch %s: %v, continuing to create it.\n", branchName, err)
	}
	// Update the hash in the SAAS file
	fileContent, err := os.ReadFile(saasFile)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", saasFile, err)
	}

	// Replace the hash in the file content
	newContent := strings.ReplaceAll(string(fileContent), oldTag, promotionTag)

	err = os.WriteFile(saasFile, []byte(newContent), 0600)
	if err != nil {
		return fmt.Errorf("failed to write to file %s: %v", saasFile, err)
	}
	return nil
}

func (a AppInterface) CommitSaasFile(saasFile, commitMessage string) error {
	// Commit the change
	if err := a.GitExecutor.Run(a.GitDirectory, "git", "add", saasFile); err != nil {
		return fmt.Errorf("failed to add file %s: %v", saasFile, err)
	}
	if err := a.GitExecutor.Run(a.GitDirectory, "git", "commit", "-m", commitMessage); err != nil {
		return fmt.Errorf("failed to commit changes: %v", err)
	}

	return nil
}
