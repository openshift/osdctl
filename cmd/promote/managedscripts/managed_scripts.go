package managedscripts

import (
	"fmt"
	"path/filepath"

	"github.com/openshift/osdctl/cmd/promote/utils"
	"github.com/spf13/cobra"

	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	serviceRelPath   = "data/services/backplane/cicd/saas/saas-backplane-api.yaml"
	prodNamespaceRef = "backplanep"
	templateRepoUrl  = "https://github.com/openshift/managed-scripts"
	templateRelPath  = "hack/00-osd-managed-cluster-config-production.yaml.tmpl"
)

type managedScriptsOptions struct {
	namespaceRef             string
	gitHash                  string
	appInterfaceProvidedPath string
}

type promoteCallbacks struct {
	utils.DefaultPromoteCallbacks
}

func (c *promoteCallbacks) FilterTargets(targetNodes []*kyaml.RNode) ([]*kyaml.RNode, error) {
	return utils.FilterTargetsContainingNamespaceRef(targetNodes, prodNamespaceRef)
}

func (*promoteCallbacks) GetResourceTemplateRepoUrl(resourceTemplateNode *kyaml.RNode) (string, error) {
	return templateRepoUrl, nil
}

func (*promoteCallbacks) GetResourceTemplateRelPath(resourceTemplateNode *kyaml.RNode) (string, error) {
	return templateRelPath, nil
}

func (c *promoteCallbacks) GetTargetHash(targetNode *kyaml.RNode) (string, error) {
	value, err := targetNode.GetString("parameters.MANAGED_SCRIPTS_GIT_SHA")
	if err != nil || value == "" {
		return "", fmt.Errorf("path 'resourceTemplates[].targets[].parameters.MANAGED_SCRIPTS_GIT_SHA' is not always defined as a non-empty string in '%s': %v", c.Service.GetFilePath(), err)
	}

	return value, nil
}

func (c *promoteCallbacks) SetTargetHash(targetNode *kyaml.RNode, newHash string) error {
	err := targetNode.PipeE(kyaml.Lookup("parameters", "MANAGED_SCRIPTS_GIT_SHA"), kyaml.Set(kyaml.NewStringRNode(newHash)))
	if err != nil {
		return fmt.Errorf("failed to set 'resourceTemplates[].targets[].parameters.MANAGED_SCRIPTS_GIT_SHA' to '%s' in '%s': %v", newHash, c.Service.GetFilePath(), err)
	}

	return nil
}

// NewCmdManagedScripts implements the command promoting https://github.com/openshift/managed-scripts
func NewCmdManagedScripts() *cobra.Command {
	ops := &managedScriptsOptions{}
	cmd := &cobra.Command{
		Use:               "managedscripts",
		Short:             "Promote https://github.com/openshift/managed-scripts",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Example: `
		# Promote managed-scripts repo
		osdctl promote managedscripts --gitHash <git-hash>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			appInterfaceClone, err := utils.FindAppInterfaceClone(ops.appInterfaceProvidedPath)
			if err != nil {
				return err
			}

			service, err := utils.ReadServiceFromFile(
				appInterfaceClone,
				filepath.Join(appInterfaceClone.GetPath(), serviceRelPath))
			if err != nil {
				return err
			}

			return service.Promote(&promoteCallbacks{
				DefaultPromoteCallbacks: utils.DefaultPromoteCallbacks{Service: service},
			}, ops.gitHash)
		},
	}

	cmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "Git hash of the managed-scripts repo commit getting promoted")
	cmd.Flags().StringVarP(&ops.namespaceRef, "namespaceRef", "n", "", "SaaS target namespace reference name")
	cmd.Flags().StringVarP(&ops.appInterfaceProvidedPath, "appInterfaceDir", "", "", "location of app-interface checkout. Falls back to current working directory")

	return cmd
}
