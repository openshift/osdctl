package dynatrace

import (
	"fmt"
	"os"

	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openshift/osdctl/cmd/promote/iexec"
	"github.com/openshift/osdctl/cmd/promote/utils"

	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	saasDynatraceDir = "data/services/osd-operators/cicd/saas/saas-dynatrace"
	moduleDir        = "terraform/modules"
	ProductionDir    = "terraform/redhat-aws/sd-sre/production"
	pattern          = "git::https://gitlab.cee.redhat.com/service/dynatrace-config.git//terraform/modules/"
)

var (
	ModulesSlice    []string
	ModulesFilesMap = map[string]string{}
)

func validateDynatraceServiceFilePath(filePath string) string {
	if !strings.HasSuffix(filePath, ".yaml") {
		return ""
	}

	return filePath
}

func getResourceTemplatesPaths(serviceRegistry *utils.ServicesRegistry, serviceId string) string {
	service, err := serviceRegistry.GetService(serviceId)
	if err != nil {
		return ""
	}

	var paths []string

	err = service.GetResourceTemplatesSequenceNode().VisitElements(func(resourceTemplateNode *kyaml.RNode) error {
		path, err := resourceTemplateNode.GetString("path")
		if err != nil || path == "" {
			return nil
		}

		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return ""
	}
	return strings.Join(paths, ", ")
}

func listServiceIds(serviceRegistry *utils.ServicesRegistry) error {
	serviceIds := serviceRegistry.GetServicesIds()

	fmt.Println("### Available Dynatrace components ###")

	// Find the longest service name for alignment
	maxLen := 0
	for _, serviceId := range serviceIds {
		if len(serviceId) > maxLen {
			maxLen = len(serviceId)
		}
	}

	for _, serviceId := range serviceIds {
		// Extract the path by parsing the YAML directly
		resourcesTemplatesPaths := getResourceTemplatesPaths(serviceRegistry, serviceId)

		// Display service name with its path
		if resourcesTemplatesPaths != "" {
			fmt.Printf("%-*s → %s\n", maxLen, serviceId, resourcesTemplatesPaths)
		} else {
			fmt.Printf("%-*s   (no specific path)\n", maxLen, serviceId)
		}
	}

	return nil
}

func listDynatraceModuleNames(dynatraceConfig DynatraceConfig) error {

	baseDir := dynatraceConfig.GitDirectory
	_, err := GetModulesNames(baseDir, moduleDir)
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

func GetModulesNames(baseDir, dir string) ([]string, error) {
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
	exec := iexec.Exec{}
	moduleFilePath := filepath.Join(basedir, moduleDir, module)
	output, err := exec.Output("", "git", "rev-list", "-n", "1", "HEAD", "--", moduleFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to get git hash: %v", err)
	}
	gitHash := strings.TrimSpace(string(output))
	fmt.Printf("The head githash for module %s is %s\n", module, gitHash)

	return gitHash, nil
}

func modulePromotion(dynatraceConfig DynatraceConfig, module string) error {
	baseDir := dynatraceConfig.GitDirectory

	_, err := GetModulesNames(baseDir, moduleDir)
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
	commitMsg := fmt.Sprintf("Promote Module %s to GitHash %s", module, promotionGitHash)

	fmt.Printf("commitMessage: %v\n", commitMsg)

	err = dynatraceConfig.commitFiles(commitMsg)

	if err != nil {
		return fmt.Errorf("failed to commit changes to app-interface; manual commit may still succeed: %w", err)
	}

	fmt.Printf("The branch %s is ready to be pushed\n", branchName)
	fmt.Println("DT service:", module)
	fmt.Println("to:", promotionGitHash)
	fmt.Println("READY TO PUSH,", module, "promotion commit is ready locally")
	return nil
}
