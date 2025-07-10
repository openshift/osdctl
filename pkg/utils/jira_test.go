package utils

import (
	"errors"
	"testing"

	"github.com/andygrunwald/go-jira"
	mocks "github.com/openshift/osdctl/pkg/utils/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetJiraIssuesForClusterWithClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockJiraClientInterface(ctrl)
	clusterID := "abc123"
	externalClusterID := "ext-abc123"

	expectedIssues := []jira.Issue{{Key: "ISSUE-1"}, {Key: "ISSUE-2"}}
	jql := `project = "OpenShift Hosted SRE Support" AND (
		"Cluster ID" ~ "ext-abc123" OR "Cluster ID" ~ "abc123" 
		OR description ~ "ext-abc123"
		OR description ~ "abc123")
		ORDER BY created DESC`

	mockClient.EXPECT().
		SearchIssues(jql).
		Return(expectedIssues, nil)

	issues, err := GetJiraIssuesForClusterWithClient(mockClient, clusterID, externalClusterID)
	assert.NoError(t, err)
	assert.Equal(t, expectedIssues, issues)
}

func TestGetJiraSupportExceptionsForOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockJiraClientInterface(ctrl)

	NewJiraClient = func(string) (JiraClientInterface, error) {
		return mockClient, nil
	}

	orgID := "123456"
	jql := `project = "Support Exceptions" AND type = Story AND Status = Approved AND
		 Resolution = Unresolved AND ("Customer Name" ~ "123456" OR "Organization ID" ~ "123456")`

	expectedIssues := []jira.Issue{{Key: "EXC-1"}}
	mockClient.EXPECT().
		SearchIssues(jql).
		Return(expectedIssues, nil)

	issues, err := GetJiraSupportExceptionsForOrg(orgID, "fake-token")
	assert.NoError(t, err)
	assert.Equal(t, expectedIssues, issues)
}

func TestCreateIssue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockJiraClientInterface(ctrl)

	mockClient.EXPECT().
		CreateIssue(gomock.Any()).
		DoAndReturn(func(i *jira.Issue) (*jira.Issue, error) {
			i.Key = "PROJ-123"
			return i, nil
		})

	createdIssue, err := CreateIssue(
		mockClient,
		"Test summary",
		"Test description",
		"Bug",
		"PROJ",
		nil, nil,
		[]string{"label1"},
	)

	assert.NoError(t, err)
	assert.Equal(t, "PROJ-123", createdIssue.Key)
}

func TestSearchIssues_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockJiraClientInterface(ctrl)
	errExpected := errors.New("Jira search failed")

	mockClient.EXPECT().
		SearchIssues(gomock.Any()).
		Return(nil, errExpected)

	_, err := mockClient.SearchIssues("dummy jql")
	assert.Equal(t, errExpected, err)
}
