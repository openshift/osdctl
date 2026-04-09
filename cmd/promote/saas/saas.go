package saas

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift/osdctl/cmd/promote/utils"
	"github.com/spf13/cobra"

	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	osdSaasDirPath = "data/services/osd-operators/cicd/saas"
	BpSaasDirPath  = "data/services/backplane/cicd/saas"
	cadSaasDirPath = "data/services/configuration-anomaly-detection/cicd"

	defaultProdTargetNameSuffix = "-prod-canary"
)

type saasOptions struct {
	list bool

	appInterfaceProvidedPath string
	serviceId                string
	gitHash                  string
	namespaceRef             string
	isHotfix                 bool
	isPKO                    bool
}

func validateSaasServiceFilePath(filePath string) string {
	if !strings.HasPrefix(filepath.Base(filePath), "saas-") {
		return ""
	}

	subFilePath := filepath.Join(filePath, "deploy.yaml")
	if fileInfo, err := os.Stat(subFilePath); err == nil && fileInfo.Mode().IsRegular() {
		return subFilePath
	}

	return filePath
}

type promoteCallbacks struct {
	utils.DefaultPromoteCallbacks

	namespaceRef string
	isHotfix     bool
	component    *utils.CodeComponent // not supposed to change on subsequent calls to ComputeCommitMessage
}

func (c *promoteCallbacks) FilterTargets(targetNodes []*kyaml.RNode) ([]*kyaml.RNode, error) {
	namespaceRef := c.namespaceRef

	if namespaceRef == "" {
		serviceNameToDefaultNamespaceRef := map[string]string{
			"saas-configuration-anomaly-detection-db": "app-sre-observability-production-int.yml",
			"saas-configuration-anomaly-detection":    "configuration-anomaly-detection-production",
			"saas-osd-rhobs-rules-and-dashboards":     "production",
			"saas-backplane-api":                      "backplanep",
		}

		namespaceRef = serviceNameToDefaultNamespaceRef[c.Service.GetName()]

		if namespaceRef == "" {
			if !c.isHotfix {
				var filteredTargetNodes []*kyaml.RNode

				// look for canary targets
				for _, targetNode := range targetNodes {
					targetName, err := targetNode.GetString("name")
					if err != nil {
						fmt.Printf("Path 'resourceTemplates[].targets[].name' is not always defined as a string in '%s': %v\n", c.Service.GetFilePath(), err)
						continue
					}
					if strings.HasSuffix(targetName, defaultProdTargetNameSuffix) {
						filteredTargetNodes = append(filteredTargetNodes, targetNode)
					}
				}

				if len(filteredTargetNodes) > 0 {
					fmt.Println("Canary targets detected!")

					return filteredTargetNodes, nil
				}
			}

			namespaceRef = utils.DefaultProdNamespaceRef
		}
	}

	return utils.FilterTargetsContainingNamespaceRef(targetNodes, namespaceRef)
}

// readE2EServiceName reads the e2e test service file to find the actual
// name field, which may differ from the operator name due to abbreviations
// or other inconsistencies.
//
// Example:
//
//	Service: saas-configure-alertmanager-operator
//	E2E service name: saas-configure-am-operator-e2e-test (abbreviated!)
//
// This function handles the inconsistency by reading the actual YAML file.
func readE2EServiceName(service *utils.Service) (string, error) {
	serviceFilePath := service.GetFilePath()
	serviceDirPath := ""
	if filepath.Base(serviceFilePath) == "deploy.yaml" {
		serviceDirPath = filepath.Dir(serviceFilePath)
	} else {
		serviceDirPath = strings.TrimSuffix(serviceFilePath, filepath.Ext(serviceFilePath))
	}

	e2eTestPath := filepath.Join(serviceDirPath, "osde2e-focus-test.yaml")

	e2eService, err := utils.ReadYamlDocFromFile(e2eTestPath)
	if err != nil {
		return "", fmt.Errorf("failed to read e2e test file: %w", err)
	}

	return e2eService.GetName(), nil
}

func computeE2EServiceName(service *utils.Service, componentName string) string {
	// Try to discover the actual e2e test service name from app-interface
	e2eServiceName, err := readE2EServiceName(service)
	if err != nil {
		// Fall back to standard naming convention
		e2eServiceName = fmt.Sprintf("saas-%s-e2e-test", componentName)
		fmt.Printf("Warning: Could not discover e2e test service name (error: %v)\n", err)
		fmt.Printf("Falling back to standard convention: %s\n", e2eServiceName)
	} else {
		fmt.Printf("Discovered e2e test service name: %s\n", e2eServiceName)
	}

	return e2eServiceName
}

// generateTestLogsURL creates a Grafana dashboard URL for viewing e2e test logs
// for the given operator and git hash.
//
// The URL includes filters for:
//   - Operator namespace/pipeline
//   - Git hash being promoted
//   - Environment (INT/STAGE)
//   - 7-day time window
//
// If discovery fails, falls back to standard naming convention.
func generateTestLogsURL(service *utils.Service, componentName, e2eServiceName, gitHash, env string) string {
	if env == "" {
		env = "osd-stage-hives02ue1"
	}

	// Grafana dashboard ID for HCM CICD Test Logs
	dashboardID := "feq1jm3omydq8c"
	baseURL := "https://grafana.app-sre.devshift.net/d"

	// Build URL with query parameters
	url := fmt.Sprintf("%s/%s/hcm-cicd-test-logs?", baseURL, dashboardID)
	url += fmt.Sprintf("var-namespace=%s-pipelines", componentName)
	url += fmt.Sprintf("&var-targetref=%s", gitHash)
	url += fmt.Sprintf("&var-env=%s", env)
	url += fmt.Sprintf("&var-saasfilename=%s", e2eServiceName)
	url += "&orgId=1"
	url += "&from=now-7d"
	url += "&to=now"
	url += "&timezone=UTC"
	url += "&var-cluster=appsrep09ue1"
	url += "&var-datasource=P7B77307D2CE073BC"
	url += "&var-loggroup=$__all"
	url += "&var-pipeline=$__all"

	return url
}

func (c *promoteCallbacks) ComputeCommitMessage(resourceTemplateRepo *utils.Repo, resourceTemplatePath, currentHash, newHash string) (*utils.CommitMessage, error) {
	commitMessage, err := c.DefaultPromoteCallbacks.ComputeCommitMessage(resourceTemplateRepo, resourceTemplatePath, currentHash, newHash)
	if err != nil {
		return nil, err
	}

	application := c.Service.GetApplication()
	if c.component == nil {
		c.component, err = application.GetComponent(resourceTemplateRepo.GetUrl())
		if err != nil {
			return nil, err
		}
	}

	if c.isHotfix {
		err = c.component.SetHotfixVersion(newHash)
		if err != nil {
			return nil, fmt.Errorf("failed to set hotfix version to '%s' for component '%s' in '%s': %v", newHash, c.component.GetName(), application.GetFilePath(), err)
		}

		err = application.Save()
		if err != nil {
			return nil, fmt.Errorf("failed to save application '%s': %v", application.GetFilePath(), err)
		}

		commitMessage.Title += " (HOTFIX; bypass progressive delivery)"
	}

	// Generate test logs URLs for INT and STAGE validation
	componentName := c.component.GetName()
	e2eServiceName := computeE2EServiceName(c.Service, componentName)
	intTestLogsURL := generateTestLogsURL(c.Service, componentName, e2eServiceName, newHash, "int")
	stageTestLogsURL := generateTestLogsURL(c.Service, componentName, e2eServiceName, newHash, "stage")

	commitMessage.TestsList += fmt.Sprintf("- 📊 [Monitor rollout status](https://inscope.corp.redhat.com/catalog/default/component/%s/rollout)\n", componentName)
	commitMessage.TestsList += fmt.Sprintf("- 🧪 [View INT e2e test logs](%s)\n", intTestLogsURL)
	commitMessage.TestsList += fmt.Sprintf("- 🧪 [View STAGE e2e test logs](%s)\n", stageTestLogsURL)
	commitMessage.TestsList += "- 🚨 [View Platform SRE Int/Stage incident activity](https://redhat.pagerduty.com/analytics/insights/incident-activity-report/9wMMqHHHSuvd8jMF1sByzA)\n"
	commitMessage.TestsList += "- 📈 [View Int/Stage PagerDuty Dashboard](https://redhat.pagerduty.com/analytics/overview-dashboard/sSWGx0MIdgVckAwpwbix8A)\n\n"

	return commitMessage, nil
}

// NewCmdSaas implements the saas command to interact with promoting SaaS services/operators
func NewCmdSaas() *cobra.Command {
	ops := &saasOptions{}
	saasCmd := &cobra.Command{
		Use:               "saas",
		Short:             "Utilities to promote SaaS services/operators",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Example: `
		# List all SaaS services/operators
		osdctl promote saas --list

		# Promote a SaaS service/operator
		osdctl promote saas --serviceId <service> --gitHash <git-hash>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appInterfaceClone, err := utils.FindAppInterfaceClone(ops.appInterfaceProvidedPath)
			if err != nil {
				return err
			}

			servicesRegistry, err := utils.NewServicesRegistry(
				appInterfaceClone,
				validateSaasServiceFilePath,
				osdSaasDirPath, BpSaasDirPath, cadSaasDirPath,
			)
			if err != nil {
				return err
			}

			if ops.list {
				if ops.serviceId != "" || ops.gitHash != "" {
					return errors.New("--list cannot be used with --serviceId or --gitHash")
				}

				fmt.Println("### Available services ###")
				for _, serviceId := range servicesRegistry.GetServicesIds() {
					fmt.Println(serviceId)
				}

				return nil
			} else {
				if ops.serviceId == "" {
					return errors.New("--serviceId is required unless --list is used")
				}

				if ops.isHotfix && ops.gitHash == "" {
					return errors.New("--hotfix requires --gitHash to be specified")
				}

				cmd.SilenceUsage = true

				serviceId := ops.serviceId
				if ops.isPKO && !strings.HasSuffix(serviceId, "-pko") {
					serviceId += "-pko"
				}

				service, err := servicesRegistry.GetService(serviceId)
				if err != nil {
					return err
				}

				return service.Promote(&promoteCallbacks{
					DefaultPromoteCallbacks: utils.DefaultPromoteCallbacks{Service: service},
					namespaceRef:            ops.namespaceRef,
					isHotfix:                ops.isHotfix,
				}, ops.gitHash)
			}
		},
	}

	saasCmd.Flags().BoolVarP(&ops.list, "list", "l", false, "List all SaaS file names (without the extension)")
	saasCmd.Flags().StringVarP(&ops.serviceId, "serviceId", "", "", "Name of the SaaS file (without the extension)")
	saasCmd.Flags().StringVarP(&ops.serviceId, "serviceName", "", "", "Name of the SaaS file (without the extension)")
	saasCmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "Git hash of the repo described by the SaaS file to promote to")
	saasCmd.Flags().StringVarP(&ops.namespaceRef, "namespaceRef", "n", "", "SaaS target namespace reference name")
	saasCmd.Flags().StringVarP(&ops.appInterfaceProvidedPath, "appInterfaceDir", "", "", "Location of app-interface checkout. Falls back to the current working directory")
	saasCmd.Flags().BoolVarP(&ops.isHotfix, "hotfix", "", false, "Add gitHash to hotfixVersions in app.yml to bypass progressive delivery (requires --gitHash)")
	saasCmd.Flags().BoolVarP(&ops.isPKO, "pko", "", false, "Promote the PKO variant of the service (appends -pko to serviceId)")
	_ = saasCmd.Flags().MarkHidden("serviceName")

	return saasCmd
}
