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

// JiraClientInterface defines the methods we use from go-jira
//
//go:generate mockgen -source=jira.go -destination=./mocks/jira_mock.go -package=utils
type JiraClientInterface interface {
	SearchIssues(jql string) ([]jira.Issue, error)
	CreateIssue(issue *jira.Issue) (*jira.Issue, error)
	User() *jira.UserService
	Issue() *jira.IssueService
	Board() *jira.BoardService
	Sprint() *jira.SprintService
}

// jiraClientWrapper wraps the actual go-jira client
type jiraClientWrapper struct {
	client *jira.Client
}

// Full implementation of the interface

func (j *jiraClientWrapper) SearchIssues(jql string) ([]jira.Issue, error) {
	issues, _, err := j.client.Issue.Search(jql, nil)
	return issues, err
}

func (j *jiraClientWrapper) CreateIssue(issue *jira.Issue) (*jira.Issue, error) {
	created, _, err := j.client.Issue.Create(issue)
	return created, err
}

func (j *jiraClientWrapper) User() *jira.UserService {
	return j.client.User
}

func (j *jiraClientWrapper) Issue() *jira.IssueService {
	return j.client.Issue
}

func (j *jiraClientWrapper) Board() *jira.BoardService {
	return j.client.Board
}

func (j *jiraClientWrapper) Sprint() *jira.SprintService {
	return j.client.Sprint
}

// Factory function
var NewJiraClient = func(jiraToken string) (JiraClientInterface, error) {
	return getJiraClient(jiraToken)
}

func getJiraClient(jiratoken string) (JiraClientInterface, error) {
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
	tp := jira.PATAuthTransport{Token: jiratoken}
	client, err := jira.NewClient(tp.Client(), JiraBaseURL)
	if err != nil {
		return nil, err
	}
	return &jiraClientWrapper{client: client}, nil
}

func GetJiraIssuesForClusterWithClient(jiraClient JiraClientInterface, clusterID, externalClusterID string) ([]jira.Issue, error) {
	jql := fmt.Sprintf(
		`project = "OpenShift Hosted SRE Support" AND (
		"Cluster ID" ~ "%[1]s" OR "Cluster ID" ~ "%[2]s" 
		OR description ~ "%[1]s"
		OR description ~ "%[2]s")
		ORDER BY created DESC`,
		externalClusterID,
		clusterID,
	)
	return jiraClient.SearchIssues(jql)
}

func GetJiraIssuesForCluster(clusterID, externalClusterID, jiratoken string) ([]jira.Issue, error) {
	client, err := NewJiraClient(jiratoken)
	if err != nil {
		return nil, fmt.Errorf("error connecting to jira: %v", err)
	}
	return GetJiraIssuesForClusterWithClient(client, clusterID, externalClusterID)
}

func GetRelatedHandoverAnnouncements(clusterID, externalClusterID, jiraToken, orgName, product string, isHCP bool, version string) ([]jira.Issue, error) {
	client, err := NewJiraClient(jiraToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create project service: %v", err)
	}

	productName := determineClusterProduct(product, isHCP)
	baseQueries := []fieldQuery{
		{Field: "Cluster ID", Value: clusterID, Operator: "~"},
		{Field: "Cluster ID", Value: externalClusterID, Operator: "~"},
	}
	jql := buildJQL(JiraHandoverAnnouncementProjectKey, baseQueries)
	issues, err := client.SearchIssues(jql)
	if err != nil {
		return nil, fmt.Errorf("failed to search for jira issues: %w", err)
	}

	extendedQueries := []fieldQuery{
		{Field: "Cluster ID", Value: "None,N/A,All", Operator: "~*"},
		{Field: "Customer Name", Value: orgName, Operator: "~"},
		{Field: "Products", Value: productName, Operator: "="},
		{Field: "affectedVersion", Value: formatVersion(version), Operator: "~"},
	}
	jql = buildJQL(JiraHandoverAnnouncementProjectKey, extendedQueries)
	otherIssues, err := client.SearchIssues(jql)
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

func GetJiraSupportExceptionsForOrg(organizationID, jiratoken string) ([]jira.Issue, error) {
	client, err := NewJiraClient(jiratoken)
	if err != nil {
		return nil, fmt.Errorf("error connecting to jira: %v", err)
	}
	jql := fmt.Sprintf(
		`project = "Support Exceptions" AND type = Story AND Status = Approved AND
		 Resolution = Unresolved AND ("Customer Name" ~ "%[1]s" OR "Organization ID" ~ "%[1]s")`,
		organizationID,
	)
	return client.SearchIssues(jql)
}

func CreateIssue(
	client JiraClientInterface,
	summary, description, ticketType, project string,
	reporter, assignee *jira.User,
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
	return client.CreateIssue(issue)
}
