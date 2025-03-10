package jira

import (
	"fmt"
	"strings"

	"github.com/andygrunwald/go-jira"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	TeamNameKey        = "jira_team"
	TeamLabelKey       = "jira_team_label"
	BoardIdLabel       = "jira_board_id"
	DefaultDescription = ""
	DefaultTicketType  = "Task"
	DefaultProject     = "OSD"
	SprintState        = "active"
	AddToSprintFlag    = "add-to-sprint"
)

var quickTaskCmd = &cobra.Command{
	Use:   "quick-task <title>",
	Short: "creates a new ticket with the given name",
	Long: `Creates a new ticket with the given name and a label specified by "jira_team_label" from the osdctl config. The flags "jira_board_id" and "jira_team" are also required for running this command.
The ticket will be assigned to the caller and added to their team's current sprint as an OSD Task.
A link to the created ticket will be printed to the console.`,
	Example: `#Create a new Jira issue
osdctl jira quick-task "Update command to take new flag"

#Create a new Jira issue and add to the caller's current sprint
osdctl jira quick-task "Update command to take new flag" --add-to-sprint
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		addToSprint, err := cmd.Flags().GetBool(AddToSprintFlag)
		if err != nil {
			return fmt.Errorf("error reading --%v flag: %w", AddToSprintFlag, err)
		}

		if !viper.IsSet(TeamNameKey) {
			return fmt.Errorf("%v value missing from config", TeamNameKey)
		}
		teamName := viper.GetString(TeamNameKey)

		if !viper.IsSet(TeamLabelKey) {
			return fmt.Errorf("%v value missing from config", TeamLabelKey)
		}
		teamLabel := viper.GetString(TeamLabelKey)

		if !viper.IsSet(BoardIdLabel) {
			return fmt.Errorf("%v value missing from config", BoardIdLabel)
		}
		boardId := viper.GetInt(BoardIdLabel)

		jiraClient, err := utils.GetJiraClient("")
		if err != nil {
			return fmt.Errorf("failed to get Jira client: %w", err)
		}

		issue, err := CreateQuickTicket(jiraClient.User, jiraClient.Issue, args[0], teamLabel)
		if err != nil {
			return fmt.Errorf("error creating ticket: %w", err)
		}
		fmt.Printf("Successfully created ticket:\n%v/browse/%v\n", utils.JiraBaseURL, issue.Key)

		if addToSprint {
			err = addTicketToCurrentSprint(jiraClient.Board, jiraClient.Sprint, issue, boardId, teamName)
			if err != nil {
				return fmt.Errorf("failed to add ticket to current sprint: %w", err)
			}
		}

		return nil
	},
}

func init() {
	quickTaskCmd.Flags().Bool("add-to-sprint", false, "whether or not to add the created Jira task to the SRE's current sprint.")
}

func CreateQuickTicket(userService *jira.UserService, issueService *jira.IssueService, summary string, teamLabel string) (*jira.Issue, error) {
	user, _, err := userService.GetSelf()
	if err != nil {
		return nil, fmt.Errorf("failed to get jira user for self: %w", err)
	}

	issue, err := utils.CreateIssue(
		issueService,
		summary,
		DefaultDescription,
		DefaultTicketType,
		DefaultProject,
		user,
		user,
		[]string{teamLabel},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create issue: %w", err)
	}

	return issue, nil
}

func addTicketToCurrentSprint(boardService *jira.BoardService, sprintService *jira.SprintService, issue *jira.Issue, boardId int, teamName string) error {
	sprints, _, err := boardService.GetAllSprintsWithOptions(boardId, &jira.GetAllSprintsOptions{State: SprintState})
	if err != nil {
		return fmt.Errorf("failed to get active sprints for board %v: %w", boardId, err)
	}

	var activeSprint jira.Sprint
	for _, sprint := range sprints.Values {
		if strings.Contains(sprint.Name, teamName) {
			activeSprint = sprint
			break
		}
	}

	_, err = sprintService.MoveIssuesToSprint(activeSprint.ID, []string{issue.ID})
	if err != nil {
		return fmt.Errorf("issue %v was not moved to active sprint: %w", issue.Key, err)
	}

	return nil
}
