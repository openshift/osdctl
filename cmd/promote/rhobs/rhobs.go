package rhobs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openshift/osdctl/pkg/promote"
	"github.com/spf13/cobra"

	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	rhobsSaasDirPath      = "data/services/rhobs/rhobs/cicd"
	rhobsProdNamespaceRef = "rhobs-production"
)

// SRE-owned saas files for monitoring stack, collection, tenant rules,
// dashboards, and synthetics. RHOBS infra saas files (alertmanager,
// thanos, loki, gateway, cache, objstore, operator) are owned by the
// RHOBS Platform Team and promoted separately.
var sreOwnedServices = map[string]bool{
	"saas-hcp-rules":                       true,
	"saas-sc-rules":                        true,
	"saas-hcp-loki-alerts":                 true,
	"saas-hcp-loki-recording-rules":        true,
	"saas-metric-collection-integration":   true,
	"saas-metric-collection-stage":         true,
	"saas-metric-collection-production":    true,
	"saas-log-forwarder-integration":       true,
	"saas-log-forwarder-stage":             true,
	"saas-log-forwarder-production":        true,
	"saas-log-event-collector-integration": true,
	"saas-log-event-collector-stage":       true,
	"saas-log-event-collector-production":  true,
	"saas-log-token-refresher-integration": true,
	"saas-log-token-refresher-stage":       true,
	"saas-log-token-refresher-production":  true,
	"saas-synthetics-agent":                true,
	"saas-synthetics-api":                  true,
	"saas-ocm-log-collection":              true,
	"saas-ocm-metric-collection":           true,
}

type rhobsOptions struct {
	list bool

	appInterfaceProvidedPath string
	configRepoPath           string
	serviceId                string
	gitHash                  string
}

func validateRhobsServiceFilePath(filePath string) string {
	base := filepath.Base(filePath)
	if !strings.HasPrefix(base, "saas-") || !strings.HasSuffix(base, ".yaml") {
		return ""
	}
	serviceId := strings.TrimSuffix(base, ".yaml")
	if !sreOwnedServices[serviceId] {
		return ""
	}
	return filePath
}

func resolveConfigRepoPath(provided string) (string, error) {
	if provided != "" {
		if _, err := os.Stat(filepath.Join(provided, ".git")); err != nil {
			return "", fmt.Errorf("invalid --configRepoDir %q: not a git repository", provided)
		}
		return provided, nil
	}
	candidates := []string{
		filepath.Join(os.Getenv("HOME"), "src", "configuration"),
		filepath.Join(os.Getenv("HOME"), "src", "rhobs-configuration"),
		filepath.Join(os.Getenv("HOME"), "src", "rhobs-configuration-gitlab"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			return p, nil
		}
	}
	return "", nil
}

type rhobsPromoteCallbacks struct {
	promote.DefaultPromoteCallbacks

	localRepoPath string
	httpsURL      string
}

func (c *rhobsPromoteCallbacks) GetResourceTemplateRepoUrl(resourceTemplateNode *kyaml.RNode) (string, error) {
	url, err := c.DefaultPromoteCallbacks.GetResourceTemplateRepoUrl(resourceTemplateNode)
	if err != nil {
		return "", err
	}
	c.httpsURL = url
	if c.localRepoPath != "" {
		return c.localRepoPath, nil
	}
	return url, nil
}

func (c *rhobsPromoteCallbacks) FilterTargets(targetNodes []*kyaml.RNode) ([]*kyaml.RNode, error) {
	return promote.FilterTargetsContainingNamespaceRef(targetNodes, rhobsProdNamespaceRef)
}

func (c *rhobsPromoteCallbacks) ComputeCommitMessage(resourceTemplateRepo *promote.Repo, resourceTemplatePath, oldHash, newHash string) (*promote.CommitMessage, error) {
	commitMessage, err := c.DefaultPromoteCallbacks.ComputeCommitMessage(resourceTemplateRepo, resourceTemplatePath, oldHash, newHash)
	if err != nil {
		return nil, err
	}
	if c.httpsURL != "" {
		commitMessage.ChangesURL = fmt.Sprintf("%s/-/compare/%s...%s", c.httpsURL, oldHash, newHash)
	}
	return commitMessage, nil
}

func isPinnedSHA(ref string) bool {
	if len(ref) != 40 {
		return false
	}
	for _, c := range ref {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func promoteAllServices(appInterfaceClone *promote.AppInterfaceClone, servicesRegistry *promote.ServicesRegistry, gitHash string) error {
	branchName := fmt.Sprintf("promote-rhobs-%s", shortHash(gitHash))
	if err := appInterfaceClone.CheckoutNewBranch(branchName); err != nil {
		return err
	}

	var promotedIds []string
	for _, id := range servicesRegistry.GetServicesIds() {
		service, err := servicesRegistry.GetService(id)
		if err != nil {
			continue
		}

		updated, err := updateProductionTargets(service, gitHash)
		if err != nil {
			fmt.Printf("Skipping %s: %v\n", id, err)
			continue
		}
		if !updated {
			continue
		}

		commitMsg := fmt.Sprintf("Promote %s to %s", id, shortHash(gitHash))
		if err := appInterfaceClone.Commit(commitMsg); err != nil {
			return fmt.Errorf("failed to commit %s: %v", id, err)
		}
		promotedIds = append(promotedIds, id)
		fmt.Printf("Promoted %s\n", id)
	}

	if len(promotedIds) == 0 {
		return errors.New("no RHOBS services had production targets to promote")
	}

	fmt.Printf("\nPromoted %d service(s) on branch: %s\n", len(promotedIds), branchName)
	fmt.Printf("Push the branch and create a MR from: %s\n", appInterfaceClone.GetPath())
	return nil
}

var shaRefPattern = regexp.MustCompile(`(\s+ref: )([0-9a-f]{40})`)

func updateProductionTargets(service *promote.Service, newHash string) (bool, error) {
	filePath := service.GetFilePath()
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to read %s: %v", filePath, err)
	}

	lines := strings.Split(string(content), "\n")
	updated := false

	for i, line := range lines {
		m := shaRefPattern.FindStringSubmatch(line)
		if m == nil || m[2] == newHash {
			continue
		}
		if targetBlockHasSubscribe(lines, i) {
			continue
		}
		lines[i] = m[1] + newHash
		updated = true
	}

	if updated {
		if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0600); err != nil {
			return false, fmt.Errorf("failed to write %s: %v", filePath, err)
		}
	}
	return updated, nil
}

func targetBlockHasSubscribe(lines []string, refLineIdx int) bool {
	for i := refLineIdx + 1; i < len(lines) && i < refLineIdx+15; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "subscribe:" {
			return true
		}
		if strings.HasPrefix(trimmed, "- name:") || strings.HasPrefix(lines[i], "- name:") {
			break
		}
	}
	return false
}

func NewCmdRhobs() *cobra.Command {
	ops := &rhobsOptions{}
	rhobsCmd := &cobra.Command{
		Use:               "rhobs",
		Short:             "Promote RHOBS configuration to production",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Example: `
		# List all RHOBS services
		osdctl promote rhobs --list

		# Promote all RHOBS services to a specific git hash
		osdctl promote rhobs --gitHash <git-hash>

		# Promote a single RHOBS service
		osdctl promote rhobs --serviceId saas-hcp-rules --gitHash <git-hash>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appInterfaceClone, err := promote.FindAppInterfaceClone(ops.appInterfaceProvidedPath)
			if err != nil {
				return err
			}

			servicesRegistry, err := promote.NewServicesRegistry(
				appInterfaceClone,
				validateRhobsServiceFilePath,
				rhobsSaasDirPath,
			)
			if err != nil {
				return err
			}

			if ops.list {
				if ops.serviceId != "" || ops.gitHash != "" {
					return errors.New("--list cannot be used with --serviceId or --gitHash")
				}

				fmt.Println("### Available RHOBS services ###")
				for _, serviceId := range servicesRegistry.GetServicesIds() {
					fmt.Println(serviceId)
				}
				return nil
			}

			cmd.SilenceUsage = true

			localRepoPath, err := resolveConfigRepoPath(ops.configRepoPath)
			if err != nil {
				return err
			}
			if localRepoPath != "" {
				fmt.Printf("Using local rhobs/configuration checkout: %s\n\n", localRepoPath)
			}

			if ops.serviceId != "" {
				service, err := servicesRegistry.GetService(ops.serviceId)
				if err != nil {
					return err
				}
				return service.Promote(&rhobsPromoteCallbacks{
					DefaultPromoteCallbacks: promote.DefaultPromoteCallbacks{Service: service},
					localRepoPath:           localRepoPath,
				}, ops.gitHash)
			}

			if ops.gitHash == "" {
				return errors.New("--gitHash is required when promoting all services")
			}
			if !isPinnedSHA(ops.gitHash) {
				return errors.New("--gitHash must be a 40-character lowercase commit SHA when promoting all services")
			}

			return promoteAllServices(appInterfaceClone, servicesRegistry, ops.gitHash)
		},
	}

	rhobsCmd.Flags().BoolVarP(&ops.list, "list", "l", false, "List all RHOBS SaaS file names")
	rhobsCmd.Flags().StringVarP(&ops.serviceId, "serviceId", "", "", "Name of the SaaS file (without extension)")
	rhobsCmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "Git hash of rhobs/configuration to promote to (required for bulk promotion; defaults to HEAD for --serviceId)")
	rhobsCmd.Flags().StringVarP(&ops.appInterfaceProvidedPath, "appInterfaceDir", "", "", "Location of app-interface checkout")
	rhobsCmd.Flags().StringVarP(&ops.configRepoPath, "configRepoDir", "", "", "Location of rhobs/configuration checkout (auto-detected from ~/src/)")

	return rhobsCmd
}
