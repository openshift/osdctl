package swarm

import (
	"fmt"
	"strings"
	"time"

	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

const (
	DefaultProject = "OHSS"
)

var (
	products = []string{"\"Openshift Dedicated\"", "\"Openshift Online Pro\"", "\"OpenShift Online Starter\"", "\"Red Hat OpenShift Service on AWS\"", "\"HyperShift Preview\""}
)

var secondaryCmd = &cobra.Command{
	Use:   "secondary",
	Short: "List unassigned JIRA issues based on criteria",
	Long: `Lists unassigned Jira issues from the 'OHSS' project
		for the following Products
		- OpenShift Dedicated
		- Openshift Online Pro
		- OpenShift Online Starter
		- Red Hat OpenShift Service on AWS
		- HyperShift Preview
		- Empty 'Products' field in Jira
		with the 'Summary' field  of the new ticket not matching the following
		- Compliance Alert
		and the 'Work Type' is not one of the RFE or Change Request `,
	Example: `#Collect tickets for secondary swarm
		osdctl swarm secondary`,
	RunE: func(cmd *cobra.Command, args []string) error {

		jiraClient, err := utils.GetJiraClient("")
		if err != nil {
			return fmt.Errorf("failed to get Jira client: %w", err)
		}

		// Print Jira IDs
		dt := time.Now()
		fmt.Print("\n")
		fmt.Println("Timestamp: ", dt.String())
		fmt.Println("Title 🠒 :Swarm: Secondary. ")
		fmt.Print("\n")
		// Build JQL query
		jql := buildJQL()

		// Search jira issues
		issues, _, err := jiraClient.Issue.Search(jql, nil)

		if err != nil {
			return fmt.Errorf("error fetching JIRA issues: %w", err)
		}
		utils.PrintJiraIssues(issues)
		return nil
	},
}

func buildJQL() string {
	builtjql := fmt.Sprintf("Project = \"%s\" AND Products in (%s)", DefaultProject, strings.Join(
		products,
		",",
	))

	builtjql += ` AND (
			(summary !~ "Compliance Alert: %" OR summary ~ "Compliance Alert: %" AND status = NEW)
			AND (status not in (Done, Resolved) AND ("Work Type" != "Request for Change (RFE)" OR "Work Type" is EMPTY) OR status in (Done, Resolved) AND ("Work Type" != "Request for Change (RFE)" OR "Work Type" is EMPTY ) AND resolutiondate > startOfDay(-2d))
			OR
			(summary !~ "Compliance Alert: %" OR summary ~ "Compliance Alert: %" AND status = NEW)
			AND (status not in (Done, Resolved) AND type != "Change Request" OR status in (Done, Resolved) AND type != "Change Request" AND resolutiondate > startOfDay(-2d))
		)`

	builtjql += " AND (status in (New, \"In Progress\")) AND assignee is EMPTY"
	builtjql += ` ORDER BY priority DESC`

	return builtjql
}
