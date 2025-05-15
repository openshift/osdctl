package git

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/openshift/osdctl/cmd/promote/iexec"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"

	"gopkg.in/yaml.v3"
)

const (
	canaryStr   = "-prod-canary"
	prodHiveStr = "hivep"
)

type Service struct {
	Name              string `yaml:"name"`
	ResourceTemplates []struct {
		Name    string `yaml:"name"`
		URL     string `yaml:"url"`
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

// replaceTargetSha replaces sha for targets in file whose name matches a given substring
// returns updated yaml, error, and false if no targets were found.
func replaceTargetSha(fileContent string, targetSuffix string, promotionGitHash string) (string, error, bool) {
	node, err := kyaml.Parse(fileContent)
	if err != nil {
		return "", fmt.Errorf("error parsing saas YAML: %v", err), false
	}
	targetFound := false
	rts, err := kyaml.Lookup("resourceTemplates").Filter(node)
	if err != nil {
		return "", fmt.Errorf("error querying resource templates: %v", err), false
	}
	for i := range len(rts.Content()) {
		targets, err := kyaml.Lookup("resourceTemplates", strconv.Itoa(i), "targets").Filter(node)
		if err != nil {
			return "", fmt.Errorf("error querying saas YAML: %v", err), false
		}
		err = targets.VisitElements(func(element *kyaml.RNode) error {
			name, _ := element.GetString("name")
			match, _ := regexp.MatchString("(.*)"+targetSuffix, name)
			if match {
				targetFound = true
				fmt.Println("updating target: ", name)
				_, err = element.Pipe(kyaml.SetField("ref", kyaml.NewStringRNode(promotionGitHash)))
				if err != nil {
					return fmt.Errorf("error setting ref: %v", err)
				}
			}
			return nil
		})
	}
	return node.MustString(), err, targetFound
}

func DefaultAppInterfaceDirectory() string {
	return filepath.Join(os.Getenv("HOME"), "git", "app-interface")
}

func BootstrapOsdCtlForAppInterfaceAndServicePromotions(appInterfaceCheckoutDir string, gitExecutor iexec.Exec) AppInterface {
	a := AppInterface{}
	a.GitExecutor = gitExecutor
	if appInterfaceCheckoutDir != "" {
		a.GitDirectory = appInterfaceCheckoutDir
		err := a.checkAppInterfaceCheckout()
		if err != nil {
			log.Fatalf("Provided directory %s is not an AppInterface directory: %v", a.GitDirectory, err)
		}
		return a
	}

	dir, err := getBaseDir(iexec.Exec{})
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
func (a *AppInterface) checkAppInterfaceCheckout() error {
	output, err := a.GitExecutor.Output(a.GitDirectory, "git", "remote", "-v")
	if err != nil {
		return fmt.Errorf("error executing 'git remote -v': %v", err)
	}

	outputString := output

	// Check if the output contains the app-interface repository URL
	if !strings.Contains(outputString, "gitlab.cee.redhat.com") && !strings.Contains(outputString, "app-interface") {
		return fmt.Errorf("not running in checkout of app-interface")
	}

	return nil
}

func GetCurrentGitHashFromAppInterface(saarYamlFile []byte, serviceName string, namespaceRef string) (string, string, error) {
	var currentGitHash string
	var serviceRepo string
	var service Service
	err := yaml.Unmarshal(saarYamlFile, &service)
	if err != nil {
		log.Fatal(fmt.Errorf("cannot unmarshal yaml data of service %s: %v", serviceName, err))
	}

	if namespaceRef != "" {
		for _, resourceTemplate := range service.ResourceTemplates {
			for _, target := range resourceTemplate.Targets {
				if strings.Contains(target.Namespace["$ref"], namespaceRef) {
					currentGitHash = target.Ref
					break
				}
			}
		}
	} else if service.Name == "saas-configuration-anomaly-detection-db" {
		for _, resourceTemplate := range service.ResourceTemplates {
			for _, target := range resourceTemplate.Targets {
				if strings.Contains(target.Namespace["$ref"], "app-sre-observability-production-int.yml") {
					currentGitHash = target.Ref
					break
				}
			}
		}
	} else if strings.Contains(service.Name, "configuration-anomaly-detection") {
		for _, resourceTemplate := range service.ResourceTemplates {
			for _, target := range resourceTemplate.Targets {
				if strings.Contains(target.Namespace["$ref"], "configuration-anomaly-detection-production") {
					currentGitHash = target.Ref
					break
				}
			}
		}
	} else if strings.Contains(service.Name, "rhobs-rules-and-dashboards") {
		for _, resourceTemplate := range service.ResourceTemplates {
			for _, target := range resourceTemplate.Targets {
				if strings.Contains(service.Name, "production") {
					currentGitHash = target.Ref
					break
				}
			}
		}
	} else if strings.Contains(service.Name, "saas-backplane-api") {
		for _, resourceTemplate := range service.ResourceTemplates {
			for _, target := range resourceTemplate.Targets {
				if strings.Contains(target.Namespace["$ref"], "backplanep") {
					currentGitHash = target.Ref
					break
				}
			}
		}
	} else {
		for _, resourceTemplate := range service.ResourceTemplates {
			if !strings.Contains(resourceTemplate.Name, "package") {
				for _, target := range resourceTemplate.Targets {
					if strings.Contains(target.Name, canaryStr) {
						currentGitHash = target.Ref // get canary target ref
						break
					}
				}
				if currentGitHash == "" { // canary targets not found
					for _, target := range resourceTemplate.Targets {
						if strings.Contains(target.Namespace["$ref"], prodHiveStr) {
							currentGitHash = target.Ref
							break
						}
					}
				}
			}
		}
	}

	if currentGitHash == "" {
		return "", "", fmt.Errorf("production namespace not found for service %s", serviceName)
	}

	if len(service.ResourceTemplates) > 0 {
		serviceRepo = service.ResourceTemplates[0].URL
	}

	if serviceRepo == "" {
		return "", "", fmt.Errorf("service repo not found for service %s", serviceName)
	}

	return currentGitHash, serviceRepo, nil
}

func GetCurrentPackageTagFromAppInterface(saasFile string) (string, error) {
	saasData, err := os.ReadFile(saasFile)
	if err != nil {
		return "", fmt.Errorf("failed to read file '%s': %w", saasFile, err)
	}

	service := Service{}
	err = yaml.Unmarshal(saasData, &service)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal service definition: %w", err)
	}

	var currentPackageTag string
	if strings.Contains(service.Name, "configuration-anomaly-detection") {
		return "", fmt.Errorf("cannot promote package for configuration-anomaly-detection")
	}
	if strings.Contains(service.Name, "rhobs-rules-and-dashboards") {
		return "", fmt.Errorf("cannot promote package for rhobs-rules-and-dashboards")
	}
	for _, resourceTemplate := range service.ResourceTemplates {
		if strings.Contains(resourceTemplate.Name, "package") {
			for _, target := range resourceTemplate.Targets {
				if strings.Contains(target.Namespace["$ref"], prodHiveStr) {
					currentPackageTag = target.Parameters["PACKAGE_TAG"].(string)
				}
			}
		}
	}
	return currentPackageTag, nil
}

func (a *AppInterface) UpdateAppInterface(_, saasFile, currentGitHash, promotionGitHash, branchName string) error {

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
	var newContent string

	// If canary targets are set up in saas, replace the hash only in canary targets in the file content
	// Otherwise proceed to promoting to all prod hives.
	newContent, err, canaryTargetsSetUp := replaceTargetSha(string(fileContent), canaryStr, promotionGitHash)
	if err != nil {
		return fmt.Errorf("error modifying YAML: %v", err)
	}
	if !canaryTargetsSetUp {
		fmt.Println("canary targets not set, continuing to replace all occurrences of sha.")
		newContent = strings.ReplaceAll(string(fileContent), currentGitHash, promotionGitHash)
	}

	err = os.WriteFile(saasFile, []byte(newContent), 0600)
	if err != nil {
		return fmt.Errorf("failed to write to file %s: %v", saasFile, err)
	}

	return nil
}

func (a *AppInterface) UpdatePackageTag(saasFile, oldTag, promotionTag, branchName string) error {

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

func (a *AppInterface) CommitSaasFile(saasFile, commitMessage string) error {
	// Commit the change
	if err := a.GitExecutor.Run(a.GitDirectory, "git", "add", saasFile); err != nil {
		return fmt.Errorf("failed to add file %s: %v", saasFile, err)
	}
	if err := a.GitExecutor.Run(a.GitDirectory, "git", "commit", "-m", commitMessage); err != nil {
		return fmt.Errorf("failed to commit changes: %v", err)
	}

	return nil
}
