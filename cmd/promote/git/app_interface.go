package git

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

type ServiceOsd struct {
	Name              string `yaml:"name"`
	ResourceTemplates []struct {
		Name    string `yaml:"name"`
		URL     string `yaml:"url"`
		Targets []struct {
			Namespace  map[string]string `yaml:"namespace"`
			Ref        string            `yaml:"ref"`
			Parameters map[string]string `yaml:"parameters"`
		} `yaml:"targets"`
	} `yaml:"resourceTemplates"`
}

type ServiceHcp struct {
	Name              string `yaml:"name"`
	ResourceTemplates []struct {
		URL     string `yaml:"url"`
		Targets []struct {
			NamespaceSelector struct {
				JSONPathSelectors struct {
					Include []string `yaml:"include"`
				} `yaml:"jsonPathSelectors"`
			} `yaml:"namespaceSelector"`
			Ref        string            `yaml:"ref"`
			Parameters map[string]string `yaml:"parameters"`
		} `yaml:"targets"`
	} `yaml:"resourceTemplates"`
}

func BootstrapOsdCtlForAppInterfaceAndServicePromotions() {
	_, err := getBaseDir()
	if err != nil {
		log.Fatal(err)
	}
	err = checkAppInterfaceCheckout()
	if err != nil {
		log.Fatal(err)
	}
}

// checkAppInterfaceCheckout checks if the script is running in the checkout of app-interface
func checkAppInterfaceCheckout() error {
	cmd := exec.Command("git", "remote", "-v")
	cmd.Dir = BaseDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error executing 'git remote -v': %v", err)
	}

	outputString := string(output)

	// Check if the output contains the app-interface repository URL
	if !strings.Contains(outputString, "gitlab.cee.redhat.com") && !strings.Contains(outputString, "app-interface") {
		return fmt.Errorf("not running in checkout of app-interface")
	}
	fmt.Println("Running in checkout of app-interface.")

	return nil
}

func GetCurrentGitHashFromAppInterface(saasYamlFile []byte, serviceName string, osd, hcp bool) (string, string, error) {
	var currentGitHash string
	var serviceRepo string
	var serviceOsd ServiceOsd
	var serviceHcp ServiceHcp

	if osd {
		err := yaml.Unmarshal(saasYamlFile, &serviceOsd)
		if err != nil {
			log.Fatal(err)
		}

		if serviceOsd.Name == "saas-configuration-anomaly-detection-db" {
			for _, resourceTemplate := range serviceOsd.ResourceTemplates {
				for _, target := range resourceTemplate.Targets {
					if strings.Contains(target.Namespace["$ref"], "app-sre-observability-production-int.yml") {
						currentGitHash = target.Ref
						break
					}
				}
			}
		} else if strings.Contains(serviceOsd.Name, "configuration-anomaly-detection") {
			for _, resourceTemplate := range serviceOsd.ResourceTemplates {
				for _, target := range resourceTemplate.Targets {
					if strings.Contains(target.Namespace["$ref"], "configuration-anomaly-detection-production") {
						currentGitHash = target.Ref
						break
					}
				}
			}
		} else if strings.Contains(serviceOsd.Name, "rhobs-rules-and-dashboards") {
			for _, resourceTemplate := range serviceOsd.ResourceTemplates {
				for _, target := range resourceTemplate.Targets {
					if strings.Contains(serviceOsd.Name, "rhobsp02ue1-production") {
						currentGitHash = target.Ref
						break
					}
				}
			}
		} else if strings.Contains(serviceOsd.Name, "rhobs-rules-and-dashboards") {
			for _, resourceTemplate := range serviceOsd.ResourceTemplates {
				for _, target := range resourceTemplate.Targets {
					if strings.Contains(serviceOsd.Name, "rhobsp02ue1-production") {
						currentGitHash = target.Ref
						break
					}
				}
			}
		} else {
			for _, resourceTemplate := range serviceOsd.ResourceTemplates {
				if !strings.Contains(resourceTemplate.Name, "package") {
					for _, target := range resourceTemplate.Targets {
						if strings.Contains(target.Namespace["$ref"], "hivep") {
							currentGitHash = target.Ref
							break
						}
					}
				}
			}
		}

		if len(serviceOsd.ResourceTemplates) > 0 {
			serviceRepo = serviceOsd.ResourceTemplates[0].URL
		}
	} else if hcp {
		err := yaml.Unmarshal(saasYamlFile, &serviceHcp)
		if err != nil {
			log.Fatal(err)
		}

		for _, resourceTemplate := range serviceHcp.ResourceTemplates {
			for _, target := range resourceTemplate.Targets {
				if strings.Contains(target.NamespaceSelector.JSONPathSelectors.Include[0], "@.name==\"production\"") && target.Parameters["REGION"] == "us-east-1" && target.Parameters["SECTOR"] == "canary" {
					currentGitHash = target.Ref
					break
				}
			}
		}

		if len(serviceHcp.ResourceTemplates) > 0 {
			serviceRepo = serviceHcp.ResourceTemplates[0].URL
		}

	}

	if currentGitHash == "" {
		return "", "", fmt.Errorf("production namespace not found for service %s", serviceName)
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

	service := ServiceOsd{}
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
				if strings.Contains(target.Namespace["$ref"], "hivep") {
					currentPackageTag = target.Parameters["PACKAGE_TAG"]
				}
			}
		}
	}
	return currentPackageTag, nil
}

func UpdateAppInterface(serviceName, saasFile, currentGitHash, promotionGitHash, branchName string) error {
	cmd := exec.Command("git", "checkout", "-b", branchName, "master")
	cmd.Dir = BaseDir
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %v", branchName, err)
	}

	// Update the hash in the SAAS file
	fileContent, err := os.ReadFile(saasFile)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", saasFile, err)
	}

	// Replace the hash in the file content
	newContent := strings.ReplaceAll(string(fileContent), currentGitHash, promotionGitHash)

	err = os.WriteFile(saasFile, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write to file %s: %v", saasFile, err)
	}

	return nil
}

func UpdatePackageTag(saasFile, oldTag, promotionTag, branchName string) error {
	cmd := exec.Command("git", "checkout", "-b", branchName, "master")
	cmd.Dir = BaseDir
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %v", branchName, err)
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

func CommitSaasFile(saasFile, commitMessage string) error {
	// Commit the change
	cmd := exec.Command("git", "add", saasFile)
	cmd.Dir = BaseDir
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to add file %s: %v", saasFile, err)
	}

	//commitMessage := fmt.Sprintf("Promote %s to %s", serviceName, promotionGitHash)
	cmd = exec.Command("git", "commit", "-m", commitMessage)
	cmd.Dir = BaseDir
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to commit changes: %v", err)
	}

	return nil
}
