package dynatrace

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
}

func DefaultAppInterfaceDirectory() string {
	return filepath.Join(os.Getenv("HOME"), "git", "app-interface")
}

func BootstrapOsdCtlForAppInterfaceAndServicePromotions(appInterfaceCheckoutDir string) AppInterface {
	a := AppInterface{}
	if appInterfaceCheckoutDir != "" {
		a.GitDirectory = appInterfaceCheckoutDir
		err := checkAppInterfaceCheckout(a.GitDirectory)
		if err != nil {
			log.Fatalf("Provided directory %s is not an AppInterface directory: %v", a.GitDirectory, err)
		}
		return a
	}

	dir, err := getBaseDir()
	if err == nil {
		a.GitDirectory = dir
		err = checkAppInterfaceCheckout(a.GitDirectory)
		if err == nil {
			return a
		}
	}

	log.Printf("Not running in AppInterface directory: %v - Trying %s next\n", err, DefaultAppInterfaceDirectory())
	a.GitDirectory = DefaultAppInterfaceDirectory()
	err = checkAppInterfaceCheckout(a.GitDirectory)
	if err != nil {
		log.Fatalf("%s is not an AppInterface directory: %v", DefaultAppInterfaceDirectory(), err)
	}

	log.Printf("Found AppInterface in %s.\n", a.GitDirectory)
	return a
}

// checkAppInterfaceCheckout checks if the script is running in the checkout of app-interface
func checkAppInterfaceCheckout(directory string) error {
	cmd := exec.Command("git", "remote", "-v")
	cmd.Dir = directory
	output, err := cmd.CombinedOutput()
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
	cmd := exec.Command("git", "checkout", "master")
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

	cmd = exec.Command("git", "checkout", "-b", branchName, "master")
	cmd.Dir = a.GitDirectory
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %v, does it already exist? If so, please delete it with `git branch -D %s` first", branchName, err, branchName)
	}

	// Update the hash in the SAAS file

	fileContent, err := os.ReadFile(saasFile)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", saasFile, err)
	}
	var newContent string

	// Replace the hash in the file content
	newContent = strings.ReplaceAll(string(fileContent), currentGitHash, promotionGitHash)

	err = os.WriteFile(saasFile, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write to file %s: %v", saasFile, err)
	}

	return nil
}

func (a AppInterface) UpdatePackageTag(saasFile, oldTag, promotionTag, branchName string) error {
	cmd := exec.Command("git", "checkout", "master")
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

	// Update the hash in the SAAS file
	fileContent, err := os.ReadFile(saasFile)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", saasFile, err)
	}

	// Replace the hash in the file content
	newContent := strings.ReplaceAll(string(fileContent), oldTag, promotionTag)

	err = os.WriteFile(saasFile, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write to file %s: %v", saasFile, err)
	}
	return nil
}

func (a AppInterface) CommitSaasFile(saasFile, commitMessage string) error {
	// Commit the change
	cmd := exec.Command("git", "add", saasFile)
	cmd.Dir = a.GitDirectory
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to add file %s: %v", saasFile, err)
	}

	cmd = exec.Command("git", "commit", "-m", commitMessage)
	cmd.Dir = a.GitDirectory
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to commit changes: %v", err)
	}

	return nil
}
