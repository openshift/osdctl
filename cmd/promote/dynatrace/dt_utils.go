package dynatrace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	saasDynatraceDir = "data/services/osd-operators/cicd/saas/saas-dynatrace"
)

var (
	ServicesSlice    []string
	ServicesFilesMap = map[string]string{}
)

func listServiceNames(appInterface AppInterface) error {
	_, err := GetServiceNames(appInterface, saasDynatraceDir)
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

func servicePromotion(appInterface AppInterface, component, gitHash string) error {

	_, err := GetServiceNames(appInterface, saasDynatraceDir)
	if err != nil {
		return err
	}

	component, err = ValidateServiceName(ServicesSlice, component)
	if err != nil {
		return err
	}

	saasDir, err := GetSaasDir(component)
	if err != nil {
		return err
	}
	fmt.Printf("SAAS Directory: %v\n", saasDir)

	serviceData, err := os.ReadFile(saasDir)
	if err != nil {
		return fmt.Errorf("failed to read SAAS file: %v", err)
	}

	currentGitHash, serviceRepo, serviceFullPath, err := GetCurrentGitHashFromAppInterface(serviceData, component)
	if err != nil {
		return fmt.Errorf("failed to get current git hash or service repo: %v", err)
	}

	fmt.Printf("Current Git Hash: %v\nGit Repo: %v\nComponent path: %v\n", currentGitHash, serviceRepo, serviceFullPath)

	promotionGitHash, commitLog, err := CheckoutAndCompareGitHash(serviceRepo, gitHash, currentGitHash, strings.TrimPrefix(serviceFullPath, "/"))
	if err != nil {
		return fmt.Errorf("failed to checkout and compare git hash: %v", err)
	} else if promotionGitHash == "" {
		fmt.Printf("Unable to find a git hash to promote. Exiting.\n")
		os.Exit(6)
	}
	fmt.Printf("Service: %s will be promoted to %s\n", component, promotionGitHash)
	fmt.Printf("commitLog: %v\n", commitLog)
	branchName := fmt.Sprintf("promote-%s-%s", component, promotionGitHash)

	err = appInterface.UpdateAppInterface(component, saasDir, currentGitHash, promotionGitHash, branchName)
	if err != nil {
		fmt.Printf("FAILURE: %v\n", err)
	}

	commitMessage := fmt.Sprintf("Promote %s to %s\n\nSee %s/compare/%s...%s for contents of the promotion.\n clog:%s", component, promotionGitHash, serviceRepo, currentGitHash, promotionGitHash, commitLog)
	err = appInterface.CommitSaasFile(saasDir, commitMessage)
	if err != nil {
		return fmt.Errorf("failed to commit changes to app-interface: %w", err)
	}
	fmt.Printf("commitMessage: %s\n", commitMessage)

	fmt.Printf("The branch %s is ready to be pushed\n", branchName)
	fmt.Println("")
	fmt.Println("DT service:", component)
	fmt.Println("from:", currentGitHash)
	fmt.Println("to:", promotionGitHash)
	fmt.Println("READY TO PUSH,", component, "promotion commit is ready locally")
	return nil
}

func GetServiceNames(appInterface AppInterface, saaDirs ...string) ([]string, error) {
	baseDir := appInterface.GitDirectory

	for _, dir := range saaDirs {
		dirGlob := filepath.Join(baseDir, dir)
		filepaths, err := filepath.Glob(dirGlob + "/*.yaml")
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
		if service == "dynatrace-"+serviceName {
			fmt.Printf("Service %s found\n", serviceName)
			return "dynatrace-" + serviceName, nil
		}
	}

	return serviceName, fmt.Errorf("service %s not found", serviceName)
}

func GetSaasDir(component string) (string, error) {
	if saasDir, ok := ServicesFilesMap[component]; ok {
		if strings.Contains(saasDir, ".yaml") {
			return saasDir, nil
		}
	}
	return "", fmt.Errorf("saas directory for service %s not found", component)
}
