package utils

import (
	"fmt"
	"path/filepath"
	"strings"

	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	DefaultProdNamespaceRef = "hivep"
)

type yamlDoc struct {
	filePath string
	rootNode *kyaml.RNode
	name     string
}

func ReadYamlDocFromFile(filePath string) (*yamlDoc, error) {
	rootNode, err := kyaml.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read or parse '%s' file: %v", filePath, err)
	}

	name, err := rootNode.GetString("name")
	if err != nil || name == "" {
		return nil, fmt.Errorf("path 'name' is not defined as a non-empty string in '%s': %v", filePath, err)
	}

	return &yamlDoc{
		filePath: filePath,
		rootNode: rootNode,
		name:     name}, nil
}

func (d *yamlDoc) GetFilePath() string {
	return d.filePath
}

func (s *yamlDoc) GetName() string {
	return s.name
}

func (d *yamlDoc) Save() error {
	return kyaml.WriteFile(d.rootNode, d.filePath)
}

type CodeComponent struct {
	filePath string
	node     *kyaml.RNode
	name     string
}

func newCodeComponent(filePath string, node *kyaml.RNode) (*CodeComponent, error) {
	name, err := node.GetString("name")
	if err != nil || name == "" {
		return nil, fmt.Errorf("path 'codeComponents[].name' is not always defined as a non-empty string in '%s': %v", filePath, err)
	}

	return &CodeComponent{
		filePath: filePath,
		node:     node,
		name:     name}, nil
}

func (c *CodeComponent) GetName() string {
	return c.name
}

func (c *CodeComponent) SetHotfixVersion(hotfixVersion string) error {
	_, err := kyaml.SetField("hotfixVersions", kyaml.NewListRNode(hotfixVersion)).Filter(c.node)
	if err != nil {
		return fmt.Errorf("failed to set 'codeComponents[].hotfixVersions' to '%s' in '%s': %v", hotfixVersion, c.filePath, err)
	}
	return nil
}

type Application struct {
	yamlDoc
	componentsSequenceNode *kyaml.RNode
}

func readApplicationFromFile(filePath string) (*Application, error) {
	yamlDoc, err := ReadYamlDocFromFile(filePath)
	if err != nil {
		return nil, err
	}

	componentsSequenceNode, err := kyaml.Lookup("codeComponents").Filter(yamlDoc.rootNode)
	if err != nil || componentsSequenceNode == nil {
		return nil, fmt.Errorf("path 'codeComponents' is not defined in '%s': %v", yamlDoc.filePath, err)
	}

	return &Application{
		yamlDoc:                *yamlDoc,
		componentsSequenceNode: componentsSequenceNode}, nil
}

func (a *Application) GetComponent(componentUrl string) (*CodeComponent, error) {
	var componentNode *kyaml.RNode

	err := a.componentsSequenceNode.VisitElements(func(visitedNode *kyaml.RNode) error {
		visitedUrl, err := visitedNode.GetString("url")
		if err != nil {
			return fmt.Errorf("path 'codeComponents[].url' is not always defined as a string in '%s': %v", a.filePath, err)
		}
		if visitedUrl == componentUrl {
			if componentNode != nil {
				return fmt.Errorf("path 'codeComponents[].url' is defined to '%s' more than once in '%s'", componentUrl, a.filePath)
			}
			componentNode = visitedNode
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate over 'codeComponents' in '%s': %v", a.filePath, err)
	}

	if componentNode == nil {
		return nil, fmt.Errorf("path 'codeComponents[].url' is never defined to '%s' in '%s'", componentUrl, a.filePath)
	}

	return newCodeComponent(a.filePath, componentNode)
}

type Service struct {
	yamlDoc
	appInterfaceClone             *AppInterfaceClone
	application                   *Application
	resourceTemplatesSequenceNode *kyaml.RNode
}

func ReadServiceFromFile(appInterfaceClone *AppInterfaceClone, filePath string) (*Service, error) {
	yamlDoc, err := ReadYamlDocFromFile(filePath)
	if err != nil {
		return nil, err
	}

	appRelFilePath, err := yamlDoc.rootNode.GetString("app.$ref")
	if err != nil || appRelFilePath == "" {
		return nil, fmt.Errorf("path 'app.$ref' is not defined as a non-empty string in '%s': %v", filePath, err)
	}

	application, err := readApplicationFromFile(filepath.Join(appInterfaceClone.GetPath(), "data", appRelFilePath))
	if err != nil {
		return nil, fmt.Errorf("failed to read application file for service '%s': %v", filePath, err)
	}

	resourceTemplatesSequenceNode, err := kyaml.Lookup("resourceTemplates").Filter(yamlDoc.rootNode)
	if err != nil || resourceTemplatesSequenceNode == nil {
		return nil, fmt.Errorf("path 'resourceTemplates' is not defined in '%s': %v", yamlDoc.filePath, err)
	}

	return &Service{
		yamlDoc:                       *yamlDoc,
		appInterfaceClone:             appInterfaceClone,
		application:                   application,
		resourceTemplatesSequenceNode: resourceTemplatesSequenceNode}, nil
}

func (s *Service) GetRootNode() *kyaml.RNode {
	return s.rootNode
}

func (s *Service) GetResourceTemplatesSequenceNode() *kyaml.RNode {
	return s.resourceTemplatesSequenceNode
}

func (s *Service) GetApplication() *Application {
	return s.application
}

type CommitMessage struct {
	Title      string
	TestsList  string
	ChangesURL string
	ChangeLog  string
}

type PromoteCallbacks interface {
	GetResourceTemplateRepoUrl(resourceTemplateNode *kyaml.RNode) (string, error)
	GetResourceTemplateRelPath(resourceTemplateNode *kyaml.RNode) (string, error)
	FilterTargets(targetNodes []*kyaml.RNode) ([]*kyaml.RNode, error)
	GetTargetHash(targetNode *kyaml.RNode) (string, error)
	SetTargetHash(targetNode *kyaml.RNode, newHash string) error
	ComputeCommitMessage(resourceTemplateRepo *Repo, resourceTemplatePath, currentHash, newHash string) (*CommitMessage, error)
}

type DefaultPromoteCallbacks struct {
	Service *Service
}

func (c *DefaultPromoteCallbacks) getResourceTemplateField(resourceTemplateNode *kyaml.RNode, fieldName string) (string, error) {
	value, err := resourceTemplateNode.GetString(fieldName)
	if err != nil || value == "" {
		return "", fmt.Errorf("path 'resourceTemplates[].%s' is not always defined as a non-empty string in '%s': %v", fieldName, c.Service.GetFilePath(), err)
	}

	return value, nil
}

func (c *DefaultPromoteCallbacks) GetResourceTemplateRepoUrl(resourceTemplateNode *kyaml.RNode) (string, error) {
	return c.getResourceTemplateField(resourceTemplateNode, "url")
}

func (c *DefaultPromoteCallbacks) GetResourceTemplateRelPath(resourceTemplateNode *kyaml.RNode) (string, error) {
	return c.getResourceTemplateField(resourceTemplateNode, "path")
}

func FilterTargetsContainingNamespaceRef(targetNodes []*kyaml.RNode, namespaceRef string) ([]*kyaml.RNode, error) {
	var filteredTargetNodes []*kyaml.RNode

	// look for targets based on their destination / namespace
	for _, targetNode := range targetNodes {
		visitedNamespaceRef, err := targetNode.GetString("namespace.$ref")
		if err != nil || visitedNamespaceRef == "" {
			fmt.Printf("Path 'resourceTemplates[].targets[].namespace.$ref' is not always defined as a non-empty string in SAAS file: %v\n", err)
			continue
		}

		if strings.Contains(visitedNamespaceRef, namespaceRef) {
			filteredTargetNodes = append(filteredTargetNodes, targetNode)
		}
	}

	return filteredTargetNodes, nil
}

func (*DefaultPromoteCallbacks) FilterTargets(targetNodes []*kyaml.RNode) ([]*kyaml.RNode, error) {
	return FilterTargetsContainingNamespaceRef(targetNodes, DefaultProdNamespaceRef)
}

func (c *DefaultPromoteCallbacks) GetTargetHash(targetNode *kyaml.RNode) (string, error) {
	value, err := targetNode.GetString("ref")
	if err != nil || value == "" {
		return "", fmt.Errorf("path 'resourceTemplates[].targets[].ref' is not always defined as a non-empty string in '%s': %v", c.Service.GetFilePath(), err)
	}

	return value, nil
}

func (c *DefaultPromoteCallbacks) SetTargetHash(targetNode *kyaml.RNode, newHash string) error {
	_, err := kyaml.SetField("ref", kyaml.NewStringRNode(newHash)).Filter(targetNode)
	if err != nil {
		return fmt.Errorf("failed to set 'resourceTemplates[].targets[].ref' to '%s' in '%s': %v", newHash, c.Service.GetFilePath(), err)
	}

	return nil
}

func (c *DefaultPromoteCallbacks) ComputeCommitMessage(resourceTemplateRepo *Repo, resourceTemplatePath, oldHash, newHash string) (*CommitMessage, error) {
	changeLog, err := resourceTemplateRepo.FormattedLog(resourceTemplatePath, oldHash, newHash)
	if err != nil {
		return nil, err
	}

	title := fmt.Sprintf("Promote %s to %s", c.Service.GetName(), newHash)
	changesURL := fmt.Sprintf("%s/compare/%s...%s", resourceTemplateRepo.GetURL(), oldHash, newHash)

	return &CommitMessage{
		Title:      title,
		ChangesURL: changesURL,
		ChangeLog:  changeLog,
	}, nil
}

type resourceTemplatePromotion struct {
	relPath     string
	oldHash     string
	targetNodes []*kyaml.RNode
}

func formatCommitMessage(commitMessage *CommitMessage) string {
	formattedMsg := commitMessage.Title + "\n\n"

	// Add monitoring and validation links section
	if commitMessage.TestsList != "" {
		formattedMsg += "## Monitoring and Validation\n\n"
		formattedMsg += commitMessage.TestsList + "\n"
	}

	// Add changes section
	formattedMsg += "## Changes\n\n"
	formattedMsg += fmt.Sprintf("[Compare changes on GitHub](%s)\n\n", commitMessage.ChangesURL)

	// Add commit log in code block for better formatting
	formattedMsg += "### Commit Log\n\n```\n"
	formattedMsg += commitMessage.ChangeLog
	formattedMsg += "\n```"

	return formattedMsg
}

func (p *resourceTemplatePromotion) promote(callbacks PromoteCallbacks, service *Service, repo *Repo, newHash string) error {
	fmt.Printf("Resource template (in repo) path: %s\n", p.relPath)
	fmt.Printf("Resource template current hash  : %v\n", p.oldHash)
	fmt.Printf("Resource template new hash      : %v\n", newHash)

	for _, targetNode := range p.targetNodes {
		err := callbacks.SetTargetHash(targetNode, newHash)
		if err != nil {
			return err
		}
	}
	err := service.Save()
	if err != nil {
		return err
	}

	commitMessage, err := callbacks.ComputeCommitMessage(repo, p.relPath, p.oldHash, newHash)
	if err != nil {
		return err
	}

	formattedCommitMessage := formatCommitMessage(commitMessage)
	err = service.appInterfaceClone.Commit(formattedCommitMessage)
	if err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("-------------    Commit message     -------------")
	fmt.Println(formattedCommitMessage)
	fmt.Println("------------- End of commit message -------------")
	fmt.Println("")

	return nil
}

func (s *Service) Promote(callbacks PromoteCallbacks, newHash string) error {
	var resourceTemplatePromotions []*resourceTemplatePromotion

	repoUrl := ""

	err := s.resourceTemplatesSequenceNode.VisitElements(func(resourceTemplateNode *kyaml.RNode) error {
		targetsSequenceNode, err := kyaml.Lookup("targets").Filter(resourceTemplateNode)
		if err != nil || targetsSequenceNode == nil {
			return fmt.Errorf("path 'resourceTemplates[].targets' is not defined in '%s': %v", s.filePath, err)
		}

		var targetNodes []*kyaml.RNode

		err = targetsSequenceNode.VisitElements(func(targetNode *kyaml.RNode) error {
			targetNodes = append(targetNodes, targetNode)
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to iterate over 'resourceTemplates[].targets' in '%s': %v", s.filePath, err)
		}
		targetNodes, err = callbacks.FilterTargets(targetNodes)
		if err != nil {
			return err
		}

		if len(targetNodes) == 0 {
			return nil
		}

		resourceTemplateRepoUrl, err := callbacks.GetResourceTemplateRepoUrl(resourceTemplateNode)
		if err != nil {
			return err
		}
		if repoUrl == "" {
			repoUrl = resourceTemplateRepoUrl
		} else if resourceTemplateRepoUrl != repoUrl {
			return fmt.Errorf("resourceTemplates[].url not always set to '%s' for the resource templates to promote in '%s'", repoUrl, s.filePath)
		}

		resourceTemplateRelPath, err := callbacks.GetResourceTemplateRelPath(resourceTemplateNode)
		if err != nil {
			return err
		}

		oldHashToTargetNodes := make(map[string][]*kyaml.RNode)

		for _, targetNode := range targetNodes {
			oldHash, err := callbacks.GetTargetHash(targetNode)
			if err != nil {
				return err
			}
			if _, ok := oldHashToTargetNodes[oldHash]; !ok {
				oldHashToTargetNodes[oldHash] = []*kyaml.RNode{}
			}
			oldHashToTargetNodes[oldHash] = append(oldHashToTargetNodes[oldHash], targetNode)
		}

		for oldHash, targetNodes := range oldHashToTargetNodes {
			resourceTemplatePromotions = append(resourceTemplatePromotions, &resourceTemplatePromotion{
				relPath:     resourceTemplateRelPath,
				oldHash:     oldHash,
				targetNodes: targetNodes})
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to iterate over 'resourceTemplates' in '%s': %v", s.filePath, err)
	}

	if len(resourceTemplatePromotions) == 0 {
		return fmt.Errorf("nothing to promote in '%s'", s.filePath)
	}

	fmt.Printf("SAAS file                       : %s\n", s.filePath)
	fmt.Printf("Resource templates repo URL     : %s\n", repoUrl)

	repo, err := GetRepo(repoUrl)
	if err != nil {
		return err
	}
	if newHash == "" {
		newHash, err = repo.GetHeadHash()

		if err != nil {
			return err
		}
	}

	serviceFileName := filepath.Base(s.filePath)
	branchName := fmt.Sprintf("promote-%s-%s", strings.TrimSuffix(serviceFileName, filepath.Ext(serviceFileName)), newHash)
	err = s.appInterfaceClone.CheckoutNewBranch(branchName)
	if err != nil {
		return err
	}

	for _, promotion := range resourceTemplatePromotions {
		err := promotion.promote(callbacks, s, repo, newHash)
		if err != nil {
			return err
		}
	}

	fmt.Println("SUCCESS!")
	fmt.Printf("Push the following branch on your fork and create a MR from it: %s\n", branchName)
	fmt.Println("")
	fmt.Printf("(reminder: the push has to be run from the following Git clone: %s)\n", s.appInterfaceClone.GetPath())

	return nil
}
