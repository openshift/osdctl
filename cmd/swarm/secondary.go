package swarm

import (
	"fmt"
	"strings"

	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

const (
	DefaultProject = "OHSS"
)

var (
	products = []string{"Openshift Dedicated", "Openshift Online Pro", "OpenShift Online Starter", "Red Hat OpenShift Service on AWS", "HyperShift Preview"}
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

		jiraClient, err := utils.GetJiraClient()
		if err != nil {
			return fmt.Errorf("failed to get Jira client: %w", err)
		}
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
	jql := fmt.Sprintf("project = \"%s\" AND Products in (\"%s\")", DefaultProject, strings.Join(
		products,
		",",
	))

	jql += ` AND (
		(summary !~ "Compliance Alert: %" OR summary ~ "Compliance Alert: %" AND status = NEW)
		AND (status not in (Done, Resolved) AND ("Work Type" != "Request for Change (RFE)" OR "Work Type" is EMPTY) OR status in (Done, Resolved) AND ("Work Type" != "Request for Change (RFE)" OR "Work Type" is EMPTY ) AND resolutiondate > startOfDay(-2d))
		OR
		(summary !~ "Compliance Alert: %" OR summary ~ "Compliance Alert: %" AND status = NEW)
		AND (status not in (Done, Resolved) AND type != "Change Request" OR status in (Done, Resolved) AND type != "Change Request" AND resolutiondate > startOfDay(-2d))
	)`

	jql += " AND (status in (New, \"In Progress\")) AND assignee is EMPTY"

	return jql
}
