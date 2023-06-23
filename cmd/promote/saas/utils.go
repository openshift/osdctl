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
	OSDSaasDir = "data/services/osd-operators/cicd/saas"
	BPSaasDir  = "data/services/backplane/cicd/saas"
	CADSaasDir = "data/services/configuration-anomaly-detection/cicd"
)

var (
	ServicesSlice    []string
	ServicesFilesMap = map[string]string{}
)

func listServiceNames() error {
	_, err := GetServiceNames(OSDSaasDir, BPSaasDir, CADSaasDir)
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

func servicePromotion(serviceName, gitHash string, osd, hcp bool) error {
	_, err := GetServiceNames(OSDSaasDir, BPSaasDir, CADSaasDir)
	if err != nil {
		return err
	}

	err = ValidateServiceName(ServicesSlice, serviceName)
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

	currentGitHash, serviceRepo, err := git.GetCurrentGitHashFromAppInterface(serviceData, serviceName, osd, hcp)
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

	branchName := fmt.Sprintf("promote-%s-%s", serviceName, promotionGitHash)
	err = git.UpdateAppInterface(serviceName, saasDir, currentGitHash, promotionGitHash, branchName)
	if err != nil {
		fmt.Printf("FAILURE: %v\n", err)
	}

	commitMessage := fmt.Sprintf("Promote %s to %s", serviceName, promotionGitHash)
	err = git.CommitSaasFile(saasDir, commitMessage)
	if err != nil {
		return fmt.Errorf("failed to commit changes to app-interface: %w", err)
	}

	fmt.Printf("The branch %s is ready to be pushed\n", branchName)
	fmt.Println("")
	fmt.Println("service:", serviceName)
	fmt.Println("from:", currentGitHash)
	fmt.Println("to:", promotionGitHash)
	fmt.Println("READY TO PUSH,", serviceName, "promotion commit is ready locally")
	return nil
}

func GetServiceNames(saaDirs ...string) ([]string, error) {
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
			ServicesSlice = append(ServicesSlice, filename)
			ServicesFilesMap[filename] = filepath
		}
	}

	return ServicesSlice, nil
}

func ValidateServiceName(serviceSlice []string, serviceName string) error {
	fmt.Printf("### Checking if service %s exists ###\n", serviceName)
	for _, service := range serviceSlice {
		if service == serviceName {
			fmt.Printf("Service %s found\n", serviceName)
			return nil
		}
	}

	return fmt.Errorf("service %s not found", serviceName)
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
