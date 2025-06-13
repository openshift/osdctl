package jira

import (
	"fmt"
	"log"
	"strings"

	jira "github.com/andygrunwald/go-jira"
	"github.com/manifoldco/promptui"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const handoverAnnoucementsProjectID = 12351820

var createHandoverAnnouncmentCmd = &cobra.Command{
	Use:   "create-handover-announcement",
	Short: "Create a new Handover announcement for SREPHOA Project",
	Run: func(cmd *cobra.Command, args []string) {
		CreateHandoverAnnouncment()
	},
}

var allowedProducts = []string{
	"OpenShift Dedicated",
	"OpenShift Dedicated on AWS",
	"OpenShift Dedicated on GCP",
	"Red Hat Openshift on AWS",
	"Red Hat Openshift on AWS with Hosted Control Planes",
}

func CreateHandoverAnnouncment() {
	jiraClient, err := utils.NewJiraClient("")
	if err != nil {
		log.Fatalf("Failed to create Jira client: %v", err)
	}

	summary := promptInput("summary", "Enter Summary/Title for the Announcment:")
	description := promptInput("description", "Enter Description for the Announcment:")
	products, err := getProducts()
	if err != nil {
		log.Fatalf("Product validation failed: %v", err)
	}
	customer := promptInput("customer", "Enter Customer Name:")
	clusterID := promptInput("cluster", "Enter Cluster ID:")
	version := promptInput("version", "Enter Affects Version (e.g. 4.16 or 4.15.32):")

	affectsVersion, err := createVersionIfNotExists(jiraClient, version)
	if err != nil {
		log.Fatalf("Could not ensure version: %v", err)
	}

	issue := jira.Issue{
		Fields: &jira.IssueFields{
			Summary:     summary,
			Description: description,
			Project:     jira.Project{Key: "SREPHOA"},
			Type:        jira.IssueType{Name: "Story"},
			AffectsVersions: []*jira.AffectsVersion{
				{Name: affectsVersion.Name},
			},
		},
	}

	// Add custom fields
	issue.Fields.Unknowns = map[string]interface{}{
		utils.ProductCustomField:      mapProducts(products),
		utils.CustomerNameCustomField: customer,
		utils.ClusterIDCustomField:    clusterID,
	}

	created, err := jiraClient.CreateIssue(&issue)
	if err != nil {
		log.Fatalf("Failed to create issue: %s", err)
	}

	fmt.Printf("Issue created successfully: %v/browse/%s\nPlease update the announcment accordingly if required\n", utils.JiraBaseURL, created.Key)
}

func getProducts() ([]string, error) {
	productInput := viper.GetString("products")
	var selected []string

	if productInput == "" {
		fmt.Println("Available products:")
		for _, p := range allowedProducts {
			fmt.Printf("  - %s\n", p)
		}
		fmt.Println()

		prompt := promptui.Prompt{
			Label: "Enter product(s), comma-separated (e.g. Product A, Product B)",
			Validate: func(input string) error {
				if strings.TrimSpace(input) == "" {
					return fmt.Errorf("input cannot be empty")
				}
				return nil
			},
		}

		var err error
		productInput, err = prompt.Run()
		if err != nil {
			log.Fatalf("Prompt failed: %v", err)
		}
	}

	raw := strings.Split(productInput, ",")
	for _, p := range raw {
		clean := strings.TrimSpace(p)
		if clean == "" {
			continue
		}
		if !containsIgnoreCase(allowedProducts, clean) {
			return nil, fmt.Errorf("invalid product: %q (must be one of: %v)", clean, allowedProducts)
		}
		selected = append(selected, clean)
	}

	return selected, nil
}

func mapProducts(products []string) []map[string]string {
	var result []map[string]string
	for _, p := range products {
		result = append(result, map[string]string{"value": p})
	}
	return result
}

func containsIgnoreCase(list []string, val string) bool {
	for _, item := range list {
		if strings.EqualFold(item, val) {
			return true
		}
	}
	return false
}

func createVersionIfNotExists(jiraClient utils.JiraClientInterface, versionName string) (*jira.AffectsVersion, error) {
	newVersion := &jira.Version{
		Name:      versionName,
		ProjectID: handoverAnnoucementsProjectID,
	}
	createdVersion, err := jiraClient.CreateVersion(newVersion)
	if err != nil {

		return nil, fmt.Errorf("failed to create version %q: %w", versionName, err)
	}
	return &jira.AffectsVersion{
		Name: createdVersion.Name,
		ID:   createdVersion.ID,
	}, nil
}

func promptInput(flagName, promptMsg string) string {
	val := viper.GetString(flagName)
	if val != "" {
		return val
	}

	prompt := promptui.Prompt{
		Label: promptMsg,
		Validate: func(input string) error {
			if strings.TrimSpace(input) == "" {
				return fmt.Errorf("input cannot be empty")
			}
			return nil
		},
	}

	result, err := prompt.Run()
	if err != nil {
		log.Fatalf("Prompt for %q failed: %v", flagName, err)
	}

	return strings.TrimSpace(result)
}
