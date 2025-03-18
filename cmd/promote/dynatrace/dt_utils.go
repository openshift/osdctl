package dynatrace

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	saasDynatraceDir = "data/services/osd-operators/cicd/saas/saas-dynatrace"
	moduleDir        = "terraform/modules"
	ProductionDir    = "terraform/redhat-aws/sd-sre/production"
	pattern          = "git::https://gitlab.cee.redhat.com/service/dynatrace-config.git//terraform/modules/"
)

var (
	ServicesSlice    []string
	ServicesFilesMap = map[string]string{}
	ModulesSlice     []string
	ModulesFilesMap  = map[string]string{}
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

func listDynatraceModuleNames(dynatraceConfig DynatraceConfig) error {

	baseDir := dynatraceConfig.GitDirectory
	_, err := GeModulesNames(baseDir, moduleDir)
	if err != nil {
		return err
	}

	sort.Strings(ModulesSlice)
	fmt.Println("### Available terraform modules in dynatrace-config ###")
	for _, module := range ModulesSlice {
		fmt.Println(module)
	}

	return nil
}

func GeModulesNames(baseDir, dir string) ([]string, error) {

	dirGlob := filepath.Join(baseDir, dir)
	filepaths, err := os.ReadDir(dirGlob)

	if err != nil {
		return nil, err
	}

	for _, filepath := range filepaths {
		if filepath.IsDir() {
			filename := filepath.Name()
			ModulesSlice = append(ModulesSlice, filename)
			ModulesFilesMap[filename] = filepath.Name()
		}
	}

	return ModulesSlice, nil
}

func ValidateModuleName(moduleName string) (string, error) {
	fmt.Printf("### Checking if service %s exists ###\n", moduleName)
	for _, service := range ModulesSlice {
		if service == moduleName {
			fmt.Printf("Module %s found in dynatrace-config\n", moduleName)
			return moduleName, nil
		}
	}

	return moduleName, fmt.Errorf("service %s not found", moduleName)
}

func updatePromotionGitHash(module string, dir string, promotionGitHash string) (string, error) {

	fmt.Printf("Iterating over directory : %s", dir)
	items, _ := os.ReadDir(dir)
	for _, item := range items {
		fmt.Println("Production tenant: ", item.Name())
		if item.IsDir() {
			subDir := filepath.Join(dir, item.Name())
			subitems, _ := os.ReadDir(subDir)
			for _, subitem := range subitems {
				if subitem.IsDir() {
					fmt.Println("Folder : ", subitem.Name())
					subDir2 := filepath.Join(subDir, subitem.Name())
					subitems2, _ := os.ReadDir(subDir2)
					for _, subitem2 := range subitems2 {
						if !subitem2.IsDir() {
							filePath := filepath.Join(subDir2, subitem2.Name())
							extension := path.Ext(filePath)
							if extension == ".tf" {
								err := updateFileContent(filePath, module, promotionGitHash)
								if err != nil {
									return "", fmt.Errorf("error while writing files %s", err)
								}
							}
						}
					}
				}
			}
		}
	}

	return "", nil
}

func updateFileContent(filePath string, module, promotionGitHash string) error {
	var filename = filePath
	file, err := Open(filename)
	if err != nil {
		fmt.Println(err)
	}

	ok := UpdateDefaultValue(file, module, promotionGitHash)
	if ok {
		err := Save(filename, file)
		if err != nil {
			return fmt.Errorf("Error while updating file %s: %s\n", filename, err)
		}
		fmt.Printf("File Updated :%s \n ", filePath)
		return nil
	}
	return nil
}

func GetProductionDir(baseDir string) string {

	dirGlob := filepath.Join(baseDir, ProductionDir)
	return dirGlob
}

func getLatestGitHash(basedir, module string) (string, error) {

	moduleFilePath := filepath.Join(basedir, moduleDir, module)
	cmd := exec.Command("git", "rev-list", "-n", "1", "HEAD", "--", moduleFilePath)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git hash: %v", err)
	}
	gitHash := strings.TrimSpace(string(output))
	fmt.Printf("The head githash for module %s is %s\n", module, gitHash)

	return gitHash, nil
}

func modulePromotion(dynatraceConfig DynatraceConfig, module string) error {

	baseDir := dynatraceConfig.GitDirectory

	_, err := GeModulesNames(baseDir, moduleDir)
	if err != nil {
		return err
	}

	module, err = ValidateModuleName(module)
	if err != nil {
		return fmt.Errorf("Module Name : %s is not valid", module)
	}
	fmt.Printf("Module Name : %s is valid", module)

	prodtenantDir := GetProductionDir(baseDir)

	promotionGitHash, err := getLatestGitHash(baseDir, module)

	if err != nil {
		return fmt.Errorf("failed to checkout and compare git hash: %v", err)
	}

	fmt.Printf("Module: %s will be promoted to %s\n", module, promotionGitHash)

	branchName := fmt.Sprintf("promote-%s-%s", module, promotionGitHash)

	err = dynatraceConfig.UpdateDynatraceConfig(module, promotionGitHash, branchName)
	if err != nil {
		return fmt.Errorf("FAILURE: %v\n", err)
	}

	promotePattern := pattern + module + "?ref=" + promotionGitHash

	_, err = updatePromotionGitHash(module, prodtenantDir, promotePattern)

	if err != nil {
		return err
	}
	commitLog := "Promote Module " + module + " to GitHash %s" + promotionGitHash
	fmt.Printf("commitLog: %v\n", commitLog)

	err = dynatraceConfig.commitFiles(commitLog)
	if err != nil {
		return fmt.Errorf("failed to commit changes to app-interface: %w", err)
	}

	fmt.Printf("The branch %s is ready to be pushed\n", branchName)
	fmt.Println("DT service:", module)
	fmt.Println("to:", promotionGitHash)
	fmt.Println("READY TO PUSH,", module, "promotion commit is ready locally")
	return nil
}
