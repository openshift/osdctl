package utils

import (
	"fmt"
	"os"

	"github.com/andygrunwald/go-jira"
	"github.com/spf13/viper"
)

const (
	JiraTokenConfigKey = "jira_token"
)

// GetJiraClient creates a jira client that connects to
// https://issues.redhat.com. To work, the jiraToken needs to be set in the
// config
func GetJiraClient(jiratoken string) (*jira.Client, error) {
	if jiratoken == "" {
		if viper.IsSet(JiraTokenConfigKey) {
			jiratoken = viper.GetString(JiraTokenConfigKey)
		}
		if os.Getenv("JIRA_API_TOKEN") != "" {
			jiratoken = os.Getenv("JIRA_API_TOKEN")
		}
		if jiratoken == "" {
			return nil, fmt.Errorf("JIRA token is not defined")
		}
	}
	tp := jira.PATAuthTransport{
		Token: jiratoken,
	}
	return jira.NewClient(tp.Client(), JiraBaseURL)
}

func GetJiraIssuesForCluster(clusterID string, externalClusterID string, jiratoken string) ([]jira.Issue, error) {
	jiraClient, err := GetJiraClient(jiratoken)
	if err != nil {
		return nil, fmt.Errorf("error connecting to jira: %v", err)
	}

	jql := fmt.Sprintf(
		`project = "OpenShift Hosted SRE Support" AND (
		"Cluster ID" ~ "%[1]s" OR "Cluster ID" ~ "%[2]s" 
		OR description ~ "%[1]s"
		OR description ~ "%[2]s")
		ORDER BY created DESC`,
		externalClusterID,
		clusterID,
	)
	issues, _, err := jiraClient.Issue.Search(jql, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to search for jira issues: %w\n", err)
	}

	return issues, nil
}

func GetRelatedHandoverAnnouncements(clusterID string, externalClusterID string, jiraToken string, orgName string, product string, isHCP bool, version string) ([]jira.Issue, error) {
	jiraClient, err := GetJiraClient(jiraToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create project service: %v", err)
	}

	projectKey := JiraHandoverAnnouncementProjectKey
	productName := determineClusterProduct(product, isHCP)
	baseQueries := []fieldQuery{
		{Field: "Cluster ID", Value: clusterID, Operator: "~"},
		{Field: "Cluster ID", Value: externalClusterID, Operator: "~"},
	}
	jql := buildJQL(projectKey, baseQueries)
	issues, _, err := jiraClient.Issue.Search(jql, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to search for jira issues: %w", err)
	}
	extededQueries := []fieldQuery{
		{Field: "Cluster ID", Value: "None,N/A,All", Operator: "~*"},
		{Field: "Customer Name", Value: orgName, Operator: "~"},
		{Field: "Products", Value: productName, Operator: "="},
		{Field: "affectedVersion", Value: formatVersion(version), Operator: "~"},
	}

	jql = buildJQL(projectKey, extededQueries)
	otherIssues, _, err := jiraClient.Issue.Search(jql, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to search for jira issues: %w", err)
	}
	seenKeys := make(map[string]bool)
	for _, i := range issues {
		seenKeys[i.Key] = true
	}
	for _, i := range otherIssues {
		if isValidMatch(i, orgName, productName, version) && !seenKeys[i.Key] {
			issues = append(issues, i)
			seenKeys[i.Key] = true
		}
	}

	return issues, nil
}

func GetJiraSupportExceptionsForOrg(organizationID string, jiratoken string) ([]jira.Issue, error) {
	jiraClient, err := GetJiraClient(jiratoken)
	if err != nil {
		return nil, fmt.Errorf("error connecting to jira: %v", err)
	}

	jql := fmt.Sprintf(
		`project = "Support Exceptions" AND type = Story AND Status = Approved AND
		 Resolution = Unresolved AND ("Customer Name" ~ "%[1]s" OR "Organization ID" ~ "%[1]s")`,
		organizationID,
	)

	issues, _, err := jiraClient.Issue.Search(jql, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to search for jira issues %w", err)
	}

	return issues, nil
}

func CreateIssue(
	service *jira.IssueService,
	summary string,
	description string,
	ticketType string,
	project string,
	reporter *jira.User,
	assignee *jira.User,
	labels []string,
) (*jira.Issue, error) {
	issue := &jira.Issue{
		Fields: &jira.IssueFields{
			Reporter:    reporter,
			Assignee:    assignee,
			Type:        jira.IssueType{Name: ticketType},
			Project:     jira.Project{Key: project},
			Description: description,
			Summary:     summary,
			Labels:      labels,
		},
	}

	createdIssue, _, err := service.Create(issue)
	if err != nil {
		return nil, fmt.Errorf("failed to create issue: %w", err)
	}

	return createdIssue, nil
}
