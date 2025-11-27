package saas

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/openshift/osdctl/cmd/promote/git"
	"github.com/openshift/osdctl/cmd/promote/iexec"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	OSDSaasDir = "data/services/osd-operators/cicd/saas"
	BPSaasDir  = "data/services/backplane/cicd/saas"
	CADSaasDir = "data/services/configuration-anomaly-detection/cicd"
)

var (
	ServicesSlice    []string
	ServicesFilesMap = map[string]string{}
)

func listServiceNames(appInterface git.AppInterface) error {
	_, err := GetServiceNames(appInterface, OSDSaasDir, BPSaasDir, CADSaasDir)
	if err != nil {
		return err
	}

	sort.Strings(ServicesSlice)
	fmt.Println("### Available service names ###")
	for _, service := range ServicesSlice {
		fmt.Println(service)
	}

	return nil
}

func servicePromotion(appInterface git.AppInterface, serviceName, gitHash string, namespaceRef string, osd, hcp, hotfix bool) error {
	_, err := GetServiceNames(appInterface, OSDSaasDir, BPSaasDir, CADSaasDir)
	if err != nil {
		return err
	}

	serviceName, err = ValidateServiceName(ServicesSlice, serviceName)
	if err != nil {
		return err
	}

	saasDir, err := GetSaasDir(serviceName, osd, hcp)
	if err != nil {
		return err
	}
	fmt.Printf("SAAS Directory: %v\n", saasDir)

	serviceData, err := os.ReadFile(saasDir)
	if err != nil {
		return fmt.Errorf("failed to read SAAS file: %v", err)
	}

	currentGitHash, serviceRepo, err := git.GetCurrentGitHashFromAppInterface(serviceData, serviceName, namespaceRef)
	if err != nil {
		return fmt.Errorf("failed to get current git hash or service repo: %v", err)
	}
	fmt.Printf("Current Git Hash: %v\nGit Repo: %v\n\n", currentGitHash, serviceRepo)

	promotionGitHash, commitLog, err := git.CheckoutAndCompareGitHash(appInterface.GitExecutor, serviceRepo, gitHash, currentGitHash, "")
	if err != nil {
		return fmt.Errorf("failed to checkout and compare git hash: %v", err)
	} else if promotionGitHash == "" {
		fmt.Printf("Unable to find a git hash to promote. Exiting.\n")
		os.Exit(6)
	}
	fmt.Printf("Service: %s will be promoted to %s\n", serviceName, promotionGitHash)

	branchName := fmt.Sprintf("promote-%s-%s", serviceName, promotionGitHash)
	err = appInterface.UpdateAppInterface(serviceName, saasDir, currentGitHash, promotionGitHash, branchName)
	if err != nil {
		fmt.Printf("FAILURE: %v\n", err)
	}

	if hotfix {
		err = updateAppYmlWithHotfix(appInterface, serviceName, promotionGitHash)
		if err != nil {
			return fmt.Errorf("failed to update app.yml with hotfix: %v", err)
		}
	}
	prefix := "saas-"
	operatorName := strings.TrimPrefix(serviceName, prefix)

	var commitMessage string
	if hotfix {
		commitMessage = fmt.Sprintf("Promote %s to %s (HOTFIX; bypass progressive delivery)\n\nMonitor rollout status here https://inscope.corp.redhat.com/catalog/default/component/%s/rollout\n\n", serviceName, promotionGitHash, operatorName)
	} else {
		commitMessage = fmt.Sprintf("Promote %s to %s\n\nMonitor rollout status here https://inscope.corp.redhat.com/catalog/default/component/%s/rollout\n\n", serviceName, promotionGitHash, operatorName)
	}
	commitMessage += fmt.Sprintf("See %s/compare/%s...%s for contents of the promotion. clog:\n\n%s", serviceRepo, currentGitHash, promotionGitHash, commitLog)

	fmt.Printf("commitMessage: %s\n", commitMessage)

	// ovverriding appInterface.GitExecuter to iexec.Exec{}
	appInterface.GitExecutor = iexec.Exec{}

	if hotfix {
		err = appInterface.CommitSaasAndAppYmlFile(saasDir, serviceName, commitMessage)
	} else {
		err = appInterface.CommitSaasFile(saasDir, commitMessage)
	}

	if err != nil {
		return fmt.Errorf("failed to commit changes to app-interface; manual commit may still succeed: %w", err)
	}

	fmt.Printf("The branch %s is ready to be pushed\n", branchName)
	fmt.Println("")
	fmt.Println("service:", serviceName)
	fmt.Println("from:", currentGitHash)
	fmt.Println("to:", promotionGitHash)
	fmt.Println("READY TO PUSH,", serviceName, "promotion commit is ready locally")
	return nil
}

func GetServiceNames(appInterface git.AppInterface, saaDirs ...string) ([]string, error) {
	baseDir := appInterface.GitDirectory

	for _, dir := range saaDirs {
		dirGlob := filepath.Join(baseDir, dir, "saas-*")
		filepaths, err := filepath.Glob(dirGlob)
		if err != nil {
			return nil, err
		}
		for _, filepath := range filepaths {
			filename := strings.TrimPrefix(filepath, baseDir+"/"+dir+"/")
			filename = strings.TrimSuffix(filename, ".yaml")
			ServicesSlice = append(ServicesSlice, filename)
			ServicesFilesMap[filename] = filepath
		}
	}

	return ServicesSlice, nil
}

func ValidateServiceName(serviceSlice []string, serviceName string) (string, error) {
	fmt.Printf("### Checking if service %s exists ###\n", serviceName)
	for _, service := range serviceSlice {
		if service == serviceName {
			fmt.Printf("Service %s found\n", serviceName)
			return serviceName, nil
		}
		if service == "saas-"+serviceName {
			fmt.Printf("Service %s found\n", serviceName)
			return "saas-" + serviceName, nil
		}
	}

	return serviceName, fmt.Errorf("service %s not found", serviceName)
}

func GetSaasDir(serviceName string, osd bool, hcp bool) (string, error) {
	if saasDir, ok := ServicesFilesMap[serviceName]; ok {
		if strings.Contains(saasDir, ".yaml") && osd {
			return saasDir, nil
		}

		// This is a hack while we migrate the rest of the operators unto Progressive Delivery
		if osd {
			saasDir = saasDir + "/deploy.yaml"
			return saasDir, nil
		} else if hcp {
			saasDir = saasDir + "/hypershift-deploy.yaml"
			return saasDir, nil
		}
	}

	return "", fmt.Errorf("saas directory for service %s not found", serviceName)
}

// sets the hotfix git sha in app.yml, adding hotfixVersions to codeComponents if it does not exist, and otherwise overwriting the existing sha value
func setHotfixVersion(fileContent string, componentName string, gitHash string) (string, error, bool) {
	node, err := kyaml.Parse(fileContent)
	if err != nil {
		return "", fmt.Errorf("error parsing app.yml: %v", err), false
	}
	componentFound := false

	codeComponents, err := kyaml.Lookup("codeComponents").Filter(node)
	if err != nil {
		return "", fmt.Errorf("error querying codeComponents: %v", err), false
	}

	for i := range len(codeComponents.Content()) {
		component, err := kyaml.Lookup("codeComponents", strconv.Itoa(i)).Filter(node)
		if err != nil {
			return "", fmt.Errorf("error querying component %d: %v", i, err), false
		}

		name, _ := component.GetString("name")
		if name == componentName {
			componentFound = true

			fmt.Printf("Found component: %s\n", name)
			fmt.Printf("Set hotfixVersions to [%s]\n", gitHash)

			listYaml := fmt.Sprintf("- %s\n", gitHash)
			listNode, err := kyaml.Parse(listYaml)
			if err != nil {
				return "", fmt.Errorf("failed to create hotfixVerions list: %v", err), false
			}

			_, err = component.Pipe(kyaml.SetField("hotfixVersions", listNode))
			if err != nil {
				return "", fmt.Errorf("error setting hotfixVersions: %v", err), false
			}
			break
		}
	}

	return node.MustString(), err, componentFound
}

// locates the corresponding app.yml file, and updates the file with the hotfix sha
func updateAppYmlWithHotfix(appInterface git.AppInterface, serviceName, gitHash string) error {
	componentName := strings.TrimPrefix(serviceName, "saas-")

	appYmlPath := filepath.Join(appInterface.GitDirectory, "data", "services", componentName, "app.yml")

	if _, err := os.Stat(appYmlPath); os.IsNotExist(err) {
		return fmt.Errorf("app.yml file not found at %s", appYmlPath)
	}

	fileContent, err := os.ReadFile(appYmlPath)
	if err != nil {
		return fmt.Errorf("failed to read app.yml file: %v", err)
	}

	newContent, err, found := setHotfixVersion(string(fileContent), componentName, gitHash)
	if err != nil {
		return fmt.Errorf("error modifying app.yml: %v", err)
	}
	if !found {
		return fmt.Errorf("component %s not found in app.yml", componentName)
	}

	err = os.WriteFile(appYmlPath, []byte(newContent), 0600)
	if err != nil {
		return fmt.Errorf("failed to write updated app.yml: %v", err)
	}

	return nil
}
