package swarm

import (
	"strings"
	"testing"
)

func TestBuildJQL(t *testing.T) {
	expectedProject := DefaultProject
	expectedProducts := strings.Join(products, ",")
	expectedJQLContains := []string{
		`Project = "` + expectedProject + `"`,
		`Products in (` + expectedProducts + `)`,
		`summary !~ "Compliance Alert: %"`,
		`status = NEW`,
		`status not in (Done, Resolved)`,
		`"Work Type" != "Request for Change (RFE)"`,
		`type != "Change Request"`,
		`resolutiondate > startOfDay(-2d)`,
		`status in (New, "In Progress")`,
		`assignee is EMPTY`,
		`ORDER BY priority DESC`,
	}

	jql := buildJQL()

	for _, expected := range expectedJQLContains {
		if !strings.Contains(jql, expected) {
			t.Errorf("JQL query does not contain expected segment: %s", expected)
		}
	}

	if !strings.HasPrefix(jql, `Project = "`+expectedProject+`"`) {
		t.Errorf("JQL query does not start with the expected project clause")
	}

	if !strings.HasSuffix(jql, `ORDER BY priority DESC`) {
		t.Errorf("JQL query does not end with the expected order clause")
	}
}
