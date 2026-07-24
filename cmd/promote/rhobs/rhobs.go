package rhobs

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	"saas-mc-rules":                        true,
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
	list   bool
	latest bool

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
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil { //nolint:gosec // G703 false positive — paths are from hardcoded candidates, not user input
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
	prodTargets, err := promote.FilterTargetsContainingNamespaceRef(targetNodes, rhobsProdNamespaceRef)
	if err != nil {
		return nil, err
	}
	var filtered []*kyaml.RNode
	for _, t := range prodTargets {
		subscribeNode, _ := kyaml.Lookup("promotion", "subscribe").Filter(t)
		if subscribeNode != nil {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered, nil
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

var configRepoURLPatterns = []string{
	"gitlab.cee.redhat.com/rhobs/configuration",
	"gitlab.cee.redhat.com:rhobs/configuration",
}

func findUpstreamRemote(repoPath string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "remote", "-v").Output()
	if err != nil {
		return "", fmt.Errorf("failed to list remotes in %s: %v", repoPath, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "(fetch)") {
			continue
		}
		for _, pattern := range configRepoURLPatterns {
			if strings.Contains(line, pattern) {
				return strings.Fields(line)[0], nil
			}
		}
	}
	return "", fmt.Errorf("no remote pointing to gitlab.cee.redhat.com/rhobs/configuration found in %s", repoPath)
}

func resolveLatestHash(repoPath string) (string, error) {
	remote, err := findUpstreamRemote(repoPath)
	if err != nil {
		return "", err
	}
	if err := exec.Command("git", "-C", repoPath, "fetch", remote, "main").Run(); err != nil {
		return "", fmt.Errorf("failed to fetch %s/main: %v", remote, err)
	}
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", remote+"/main").Output() //nolint:gosec // G204 — remote is from findUpstreamRemote which validates against configRepoURLPatterns
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s/main: %v", remote, err)
	}
	hash := strings.TrimSpace(string(out))
	if !isPinnedSHA(hash) {
		return "", fmt.Errorf("resolved hash %q is not a valid 40-char SHA", hash)
	}
	fmt.Printf("Fetched %s/main from %s\n", remote, repoPath)
	return hash, nil
}

func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func promoteAllServices(appInterfaceClone *promote.AppInterfaceClone, servicesRegistry *promote.ServicesRegistry, gitHash, configRepoPath string) error {
	branchName := fmt.Sprintf("promote-rhobs-%s", shortHash(gitHash))
	if err := appInterfaceClone.CheckoutNewBranch(branchName); err != nil {
		return err
	}

	oldHash := getCurrentProductionHash(servicesRegistry)

	changeLog := ""
	if configRepoPath != "" && oldHash != "" {
		out, err := exec.Command("git", "-C", configRepoPath, "log", "--oneline", "--no-merges", oldHash+".."+gitHash).Output() //nolint:gosec // G204 — oldHash and gitHash are validated 40-char hex SHAs
		if err != nil {
			return fmt.Errorf("failed to generate changelog (%s..%s): %v", shortHash(oldHash), shortHash(gitHash), err)
		}
		changeLog = strings.TrimSpace(string(out))
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

		promotedIds = append(promotedIds, id)
		fmt.Printf("Promoted %s\n", id)
	}

	if len(promotedIds) == 0 {
		return errors.New("no RHOBS services had production targets to promote")
	}

	changesURL := fmt.Sprintf("https://gitlab.cee.redhat.com/rhobs/configuration/-/commit/%s", gitHash)
	if oldHash != "" {
		changesURL = fmt.Sprintf("https://gitlab.cee.redhat.com/rhobs/configuration/-/compare/%s...%s", oldHash, gitHash)
	}

	formattedMsg := fmt.Sprintf("Promote RHOBS configuration to %s\n\n", gitHash)
	formattedMsg += "## Changes\n\n"
	formattedMsg += fmt.Sprintf("[Compare changes](%s)\n\n", changesURL)
	formattedMsg += "### Commit Log\n\n```\n" + changeLog + "\n```"

	if err := appInterfaceClone.Commit(formattedMsg); err != nil {
		return fmt.Errorf("failed to commit: %v", err)
	}

	fmt.Printf("\nPromoted %d service(s) on branch: %s\n", len(promotedIds), branchName)
	fmt.Printf("Push the branch and create a MR from: %s\n", appInterfaceClone.GetPath())
	return nil
}

func getCurrentProductionHash(registry *promote.ServicesRegistry) string {
	service, err := registry.GetService("saas-hcp-rules")
	if err != nil {
		return ""
	}
	content, err := os.ReadFile(service.GetFilePath())
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(content), "\n") {
		m := shaRefPattern.FindStringSubmatch(line)
		if m != nil {
			return m[2]
		}
	}
	return ""
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
		if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0600); err != nil { //nolint:gosec // G703 false positive — filePath is from app-interface checkout, not user input
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

		# Promote all RHOBS services to the latest rhobs/configuration main
		osdctl promote rhobs --latest

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
				if ops.serviceId != "" || ops.gitHash != "" || ops.latest {
					return errors.New("--list cannot be used with --serviceId, --gitHash, or --latest")
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

			if ops.latest {
				if ops.gitHash != "" {
					return errors.New("--latest cannot be used with --gitHash")
				}
				if localRepoPath == "" {
					return errors.New("--latest requires a local rhobs/configuration checkout (set --configRepoDir or clone to ~/src/configuration)")
				}
				hash, err := resolveLatestHash(localRepoPath)
				if err != nil {
					return err
				}
				ops.gitHash = hash
				fmt.Printf("Resolved latest: %s\n\n", shortHash(hash))
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

			return promoteAllServices(appInterfaceClone, servicesRegistry, ops.gitHash, localRepoPath)
		},
	}

	rhobsCmd.Flags().BoolVarP(&ops.list, "list", "l", false, "List all RHOBS SaaS file names")
	rhobsCmd.Flags().BoolVar(&ops.latest, "latest", false, "Promote all services to the latest rhobs/configuration origin/main HEAD")
	rhobsCmd.Flags().StringVarP(&ops.serviceId, "serviceId", "", "", "Name of the SaaS file (without extension)")
	rhobsCmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "Git hash of rhobs/configuration to promote to (required for bulk promotion; defaults to HEAD for --serviceId)")
	rhobsCmd.Flags().StringVarP(&ops.appInterfaceProvidedPath, "appInterfaceDir", "", "", "Location of app-interface checkout")
	rhobsCmd.Flags().StringVarP(&ops.configRepoPath, "configRepoDir", "", "", "Location of rhobs/configuration checkout (auto-detected from ~/src/)")

	return rhobsCmd
}
