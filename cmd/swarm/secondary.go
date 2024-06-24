package swarm

import (
	"fmt"
	"os"
	"strings"
	"github.com/andygrunwald/go-jira"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	project  string
	products []string
)

var secondary = &cobra.Command{
		Use:   "secondary",
		Short: "List unassigned JIRA issues based on criteria",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate required flag
			if project == "" {
				return fmt.Errorf("project flag (-p) is required")
			}

			client, err := jira.NewClient(jira.Options{BaseURL: "https://issues.redhat.com"})
			if err != nil {
				return fmt.Errorf("Error creating JIRA client: %w", err)
			}

			jql := buildJQL()
			issues, err := client.SearchIssues(jql, nil)
			if err != nil {
				return fmt.Errorf("Error fetching JIRA issues: %w", err)
			}

			for _, issue := range issues {
				fmt.Printf("- [%s](https://issues.redhat.com/browse/%s) - [%s] - %s\n", issue.Fields.Key, issue.Fields.Key, issue.Fields.Priority.Name, issue.Fields.Summary)
			}

			return nil
		},
	}

	rootCmd.Flags().StringVarP(&project, "project", "p", "", "Project key to filter issues (required)")
	rootCmd.MarkFlagRequired("project")

	rootCmd.Flags().StringSliceVarP(&products, "products", "P", []string{"OD", "OPro", "OOS", "RHOSAWS", "HSPreview"}, "Comma-separated list of products to filter issues")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}


func buildJQL() string {
	jql := fmt.Sprintf("project = %s AND Products in (%s)", project, strings.Join(products, ","))

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

