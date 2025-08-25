package utils

import (
	"runtime/debug"
	"strings"
	"testing"

	"github.com/andygrunwald/go-jira"
)

func mockReadBuildInfo(parseBuildInfoError bool) func() (info *debug.BuildInfo, ok bool) {
	return func() (*debug.BuildInfo, bool) {
		info := &debug.BuildInfo{
			Deps: []*debug.Module{
				{
					Path:    "foo",
					Version: "v1.2.3",
				},
				{
					Path:    "bar",
					Version: "v4.5.6",
				},
			},
		}
		return info, !parseBuildInfoError
	}
}

func TestGetDependencyVersion(t *testing.T) {
	tests := []struct {
		name                string
		dependencyPath      string
		parseBuildInfoError bool
		want                string
		wantErr             bool
	}{
		{
			name:                "Error parsing build info",
			parseBuildInfoError: true,
			wantErr:             true,
		},
		{
			name:           "Dependency not found",
			dependencyPath: "test",
			wantErr:        true,
		},
		{
			name:           "Finds and returns version successfully (1)",
			dependencyPath: "foo",
			want:           "v1.2.3",
		},
		{
			name:           "Finds and returns version successfully (2)",
			dependencyPath: "bar",
			want:           "v4.5.6",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ReadBuildInfo = mockReadBuildInfo(tt.parseBuildInfoError)
			got, err := GetDependencyVersion(tt.dependencyPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetDependencyVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetDependencyVersion() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetermineClusterProduct(t *testing.T) {
	tests := []struct {
		productID string
		isHCP     bool
		expected  string
	}{
		{"rosa", true, "Red Hat OpenShift on AWS with Hosted Control Planes"},
		{"rosa", false, "Red Hat OpenShift on AWS"},
		{"osd", false, "OpenShift Dedicated"},
		{"unknown", false, ""},
	}

	for _, tt := range tests {
		got := determineClusterProduct(tt.productID, tt.isHCP)
		if got != tt.expected {
			t.Errorf("determineClusterProduct(%q, %v) = %q; want %q", tt.productID, tt.isHCP, got, tt.expected)
		}
	}
}

func TestBuildJQL(t *testing.T) {
	filters := []fieldQuery{
		{"Summary", "~*", "foo,bar"},
		{"Component", "in", `"UI","Backend"`},
		{"Severity", "=", "High"},
	}

	expectedSubstrs := []string{
		`"Summary" ~ "foo"`,
		`"Summary" ~ "bar"`,
		`"Component" in ("UI","Backend")`,
		`"Severity" = "High"`,
		`status != Closed`,
	}

	jql := buildJQL("TEST", filters)

	for _, substr := range expectedSubstrs {
		if !strings.Contains(jql, substr) {
			t.Errorf("JQL missing expected substring: %q", substr)
		}
	}
}

func TestFormatVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"4.15.3", "4.15"},
		{"4.11", "4.11"},
		{"5", "5"},
		{"4.12.0.1", "4.12"},
	}

	for _, tt := range tests {
		got := formatVersion(tt.input)
		if got != tt.expected {
			t.Errorf("formatVersion(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestIsValidMatch(t *testing.T) {
	mockIssue := func(productVal, versionName, customerName string) jira.Issue {
		return jira.Issue{
			Fields: &jira.IssueFields{
				AffectsVersions: []*jira.AffectsVersion{{Name: versionName}},
				Unknowns: map[string]interface{}{
					ProductCustomField: []interface{}{
						map[string]interface{}{"value": productVal},
					},
					CustomerNameCustomField: customerName,
				},
			},
		}
	}

	tests := []struct {
		issue      jira.Issue
		org        string
		product    string
		version    string
		shouldPass bool
	}{
		{mockIssue("Red Hat OpenShift on AWS", "4.15.3", "Acme Corp"), "Acme Corp", "Red Hat OpenShift on AWS", "4.15", true},
		{mockIssue("Red Hat OpenShift on AWS", "none", "Acme Corp"), "Acme Corp", "Red Hat OpenShift on AWS", "4.15", true},
		{mockIssue("Red Hat OpenShift on AWS", "4.15.3", "N/A"), "Acme Corp", "Red Hat OpenShift on AWS", "4.15", true},
		{mockIssue("Wrong Product", "4.15.3", "Acme Corp"), "Acme Corp", "Red Hat OpenShift on AWS", "4.15", false},
	}

	for i, tt := range tests {
		got := isValidMatch(tt.issue, tt.org, tt.product, tt.version)
		if got != tt.shouldPass {
			t.Errorf("Test %d failed: isValidMatch() = %v; want %v", i, got, tt.shouldPass)
		}
	}

}
