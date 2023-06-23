package saas

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openshift/osdctl/cmd/promote/git"
)

const (
	osdSaasDir = "data/services/osd-operators/cicd/saas"
	bpSaasDir  = "data/services/backplane/cicd/saas"
	cadSaasDir = "data/services/configuration-anomaly-detection/cicd"
)

var (
	servicesSlice    []string
	servicesFilesMap = map[string]string{}
)

func listServiceNames() error {
	_, err := getServiceNames(osdSaasDir, bpSaasDir, cadSaasDir)
	if err != nil {
		return err
	}

	sort.Strings(servicesSlice)
	fmt.Println("### Available service names ###")
	for _, service := range servicesSlice {
		fmt.Println(service)
	}

	return nil
}

func servicePromotion(serviceName, gitHash string, osd, hcp bool) error {
	_, err := getServiceNames(osdSaasDir, bpSaasDir, cadSaasDir)
	if err != nil {
		return err
	}

	err = validateServiceName(servicesSlice, serviceName)
	if err != nil {
		return err
	}

	saasDir, err := getSaasDir(serviceName, osd, hcp)
	if err != nil {
		return err
	}
	fmt.Printf("SAAS Directory: %v\n", saasDir)

	serviceData, err := os.ReadFile(saasDir)
	if err != nil {
		return fmt.Errorf("failed to read SAAS file: %v", err)
	}

	currentGitHash, serviceRepo, err := git.GetCurrentGitHashFromAppInterface(serviceData, serviceName)
	if err != nil {
		return fmt.Errorf("failed to get current git hash or service repo: %v", err)
	}
	fmt.Printf("Current Git Hash: %v\nGit Repo: %v\n\n", currentGitHash, serviceRepo)

	promotionGitHash, err := git.CheckoutAndCompareGitHash(serviceRepo, gitHash, currentGitHash)
	if err != nil {
		return fmt.Errorf("failed to checkout and compare git hash: %v", err)
	} else if promotionGitHash == "" {
		fmt.Printf("Unable to find a git hash to promote. Exiting.\n")
		os.Exit(6)
	}
	fmt.Printf("Service: %s will be promoted to %s\n", serviceName, promotionGitHash)

	err = git.UpdateAndCommitChangesForAppInterface(serviceName, saasDir, currentGitHash, promotionGitHash)
	if err != nil {
		fmt.Printf("FAILURE: %v\n", err)
	}

	return nil
}

func getServiceNames(saaDirs ...string) ([]string, error) {
	baseDir := git.BaseDir
	for _, dir := range saaDirs {
		dirGlob := filepath.Join(baseDir, dir, "saas-*")
		filepaths, err := filepath.Glob(dirGlob)
		if err != nil {
			return nil, err
		}
		for _, filepath := range filepaths {
			filename := strings.TrimPrefix(filepath, baseDir+"/"+dir+"/")
			filename = strings.TrimSuffix(filename, ".yaml")
			servicesSlice = append(servicesSlice, filename)
			servicesFilesMap[filename] = filepath
		}
	}

	return servicesSlice, nil
}

func validateServiceName(serviceSlice []string, serviceName string) error {
	fmt.Printf("### Checking if service %s exists ###\n", serviceName)
	for _, service := range serviceSlice {
		if service == serviceName {
			fmt.Printf("Service %s found\n", serviceName)
			return nil
		}
	}

	return fmt.Errorf("service %s not found", serviceName)
}

func getSaasDir(serviceName string, osd bool, hcp bool) (string, error) {
	if saasDir, ok := servicesFilesMap[serviceName]; ok {
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
