package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/andygrunwald/go-jira"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	v2 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	"github.com/openshift/osdctl/pkg/provider/pagerduty"
	"github.com/stretchr/testify/assert"
)

type MockOCMClient struct{}

type MockCluster struct {
	ID                string
	ExternalID        string
	InfraID           string
	Name              string
	CreationTimestamp time.Time
	HypershiftEnabled bool
}

func TestNewCmdContext(t *testing.T) {
	cmd := newCmdContext()

	assert.NotNil(t, cmd)
	assert.Equal(t, "context", cmd.Use)
	assert.Equal(t, "Shows the context of a specified cluster", cmd.Short)
	err := cmd.Args(cmd, []string{"cluster-id"})
	assert.NoError(t, err)
	err = cmd.Args(cmd, []string{})
	assert.Error(t, err)

	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("output"))
	assert.NotNil(t, flags.Lookup("profile"))
	assert.NotNil(t, flags.Lookup("days"))
	assert.NotNil(t, flags.Lookup("pages"))

	output, _ := cmd.Flags().GetString("output")
	assert.Equal(t, "long", output)

	days, _ := cmd.Flags().GetInt("days")
	assert.Equal(t, 30, days)

	pages, _ := cmd.Flags().GetInt("pages")
	assert.Equal(t, 40, pages)
}

func TestNewContextOptions(t *testing.T) {
	opts := newContextOptions()
	assert.NotNil(t, opts)
}

func TestPrintClusterHeader(t *testing.T) {
	data := &contextData{
		ClusterName: "test-cluster",
		ClusterID:   "12345",
	}

	output := captureOutput(func(w io.Writer) {
		data.printClusterHeader(w)
	})

	expectedHeader := fmt.Sprintf("%s -- %s", data.ClusterName, data.ClusterID)
	expectedOutput := fmt.Sprintf("%s\n%s\n%s\n",
		strings.Repeat("=", len(expectedHeader)),
		expectedHeader,
		strings.Repeat("=", len(expectedHeader)))

	if string(output) != expectedOutput {
		t.Errorf("Expected output:\n%s\nGot:\n%s", expectedOutput, string(output))
	}
}

func TestPrintDynatraceResources(t *testing.T) {
	data := &contextData{
		DyntraceEnvURL:  "https://dynatrace.com/env",
		DyntraceLogsURL: "https://dynatrace.com/logs",
	}

	output := captureOutput(func(w io.Writer) {
		printDynatraceResources(data, w)
	})

	expectedHeader := "Dynatrace Details"
	expectedLines := []string{
		"Dynatrace Tenant URL   https://dynatrace.com/env",
		"Logs App URL           https://dynatrace.com/logs",
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, expectedHeader) {
		t.Errorf("Expected output to contain header:\n%s\nGot:\n%s", expectedHeader, outputStr)
	}

	for _, expectedLine := range expectedLines {
		if !strings.Contains(outputStr, expectedLine) {
			t.Errorf("Expected output to contain:\n%s\nGot:\n%s", expectedLine, outputStr)
		}
	}
}

func TestSkippableEvent(t *testing.T) {
	testCases := []struct {
		eventName string
		expected  bool
	}{
		{"GetUser", true},
		{"ListBuckets", true},
		{"DescribeInstances", true},
		{"AssumeRoleWithSAML", true},
		{"EncryptData", true},
		{"DecryptKey", true},
		{"LookupEventsForUser", true},
		{"GenerateDataKeyPair", true},
		{"UpdateUser", false},
		{"DeleteInstance", false},
		{"CreateBucket", false},
	}

	for _, tc := range testCases {
		result := skippableEvent(tc.eventName)
		if result != tc.expected {
			t.Errorf("For event '%s', expected %v but got %v", tc.eventName, tc.expected, result)
		}
	}
}

func TestPrintCloudTrailLogs(t *testing.T) {
	eventId1 := "12345"
	eventName1 := "CreateInstance"
	username1 := "test-user"
	eventTime1 := time.Now()

	eventId2 := "67890"
	eventName2 := "DeleteBucket"
	eventTime2 := time.Now()

	events := []*types.Event{
		{
			EventId:   &eventId1,
			EventName: &eventName1,
			Username:  &username1,
			EventTime: &eventTime1,
		},
		{
			EventId:   &eventId2,
			EventName: &eventName2,
			Username:  nil,
			EventTime: &eventTime2,
		},
	}

	output := captureOutput(func(w io.Writer) {
		printCloudTrailLogs(events, w)
	})

	outputStr := string(output)

	if !strings.Contains(outputStr, "Potentially interesting CloudTrail events") {
		t.Errorf("Expected output to contain the log header, but got:\n%s", outputStr)
	}

	if !strings.Contains(outputStr, "12345") || !strings.Contains(outputStr, "CreateInstance") || !strings.Contains(outputStr, "test-user") {
		t.Errorf("Expected event details missing from output:\n%s", outputStr)
	}

	if !strings.Contains(outputStr, "67890") || !strings.Contains(outputStr, "DeleteBucket") {
		t.Errorf("Expected second event details missing from output:\n%s", outputStr)
	}
}

func (m *MockCluster) ToV1Cluster() *v1.Cluster {
	cluster, _ := v1.NewCluster().
		ID(m.ID).
		ExternalID(m.ExternalID).
		InfraID(m.InfraID).
		Name(m.Name).
		CreationTimestamp(m.CreationTimestamp).
		Hypershift(v1.NewHypershift().Enabled(m.HypershiftEnabled)).
		Build()
	return cluster
}

func TestBuildSplunkURL(t *testing.T) {
	testCases := []struct {
		name              string
		hypershiftEnabled bool
		ocmEnv            string
		clusterID         string
		clusterName       string
		infraID           string
		expectedURL       string
	}{
		{
			name:              "Hypershift enabled, production environment",
			hypershiftEnabled: true,
			ocmEnv:            "production",
			clusterID:         "mock-cluster-id",
			clusterName:       "mock-cluster",
			expectedURL:       fmt.Sprintf(HCPSplunkURL, "openshift_managed_hypershift_audit", "production", "mock-cluster-id", "mock-cluster"),
		},
		{
			name:              "Hypershift enabled, stage environment",
			hypershiftEnabled: true,
			ocmEnv:            "stage",
			clusterID:         "mock-cluster-id",
			clusterName:       "mock-cluster",
			expectedURL:       fmt.Sprintf(HCPSplunkURL, "openshift_managed_hypershift_audit_stage", "staging", "mock-cluster-id", "mock-cluster"),
		},
		{
			name:              "Hypershift enabled, unknown environment",
			hypershiftEnabled: true,
			ocmEnv:            "unknown",
			clusterID:         "mock-cluster-id",
			clusterName:       "mock-cluster",
			expectedURL:       "",
		},
		{
			name:              "Classic OpenShift, production environment",
			hypershiftEnabled: false,
			ocmEnv:            "production",
			infraID:           "mock-infra-id",
			expectedURL:       fmt.Sprintf(ClassicSplunkURL, "openshift_managed_audit", "mock-infra-id"),
		},
		{
			name:              "Classic OpenShift, stage environment",
			hypershiftEnabled: false,
			ocmEnv:            "stage",
			infraID:           "mock-infra-id",
			expectedURL:       fmt.Sprintf(ClassicSplunkURL, "openshift_managed_audit_stage", "mock-infra-id"),
		},
		{
			name:              "Classic OpenShift, unknown environment",
			hypershiftEnabled: false,
			ocmEnv:            "unknown",
			infraID:           "mock-infra-id",
			expectedURL:       "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCluster := &MockCluster{
				ID:                tc.clusterID,
				Name:              tc.clusterName,
				HypershiftEnabled: tc.hypershiftEnabled,
				CreationTimestamp: time.Now(),
			}

			o := &contextOptions{
				cluster: mockCluster.ToV1Cluster(),
				infraID: tc.infraID,
			}

			data := &contextData{
				OCMEnv: tc.ocmEnv,
			}

			actualURL := o.buildSplunkURL(data)
			assert.Equal(t, tc.expectedURL, actualURL, "Generated Splunk URL does not match expected value")
		})
	}
}

func TestPrintOtherLinks(t *testing.T) {

	mockClusterID := "mock-cluster-id"
	mockExternalClusterID := "mock-external-cluster-id"
	mockPDServiceID := []string{"PD12345"}

	o := &contextOptions{
		clusterID:         mockClusterID,
		externalClusterID: mockExternalClusterID,
	}

	data := &contextData{
		pdServiceID: mockPDServiceID,
	}

	output := captureOutput(func(w io.Writer) {
		o.printOtherLinks(data, w)
	})

	expectedLinks := []string{
		"OHSS Cards",
		"CCX dashboard",
		"Splunk Audit Logs",
		"PagerDuty Service PD12345",
	}

	for _, link := range expectedLinks {
		assert.Contains(t, output, link, "Output should contain expected link: %s", link)
	}
}

func TestPrintJIRASupportExceptions(t *testing.T) {

	mockIssues := []jira.Issue{
		{
			Key: "JIRA-123",
			Fields: &jira.IssueFields{
				Type:     jira.IssueType{Name: "Bug"},
				Priority: &jira.Priority{Name: "High"},
				Summary:  "Mock issue summary",
				Status:   &jira.Status{Name: "Open"},
			},
		},
	}

	output := captureOutput(func(w io.Writer) {
		printJIRASupportExceptions(mockIssues, w)
	})

	expectedStrings := []string{
		"- Link: https://issues.redhat.com/browse/JIRA-123",
	}

	for _, expected := range expectedStrings {
		assert.Contains(t, output, expected, "Output should contain expected text: %s", expected)
	}
}

func TestPrintHistoricalPDAlertSummary(t *testing.T) {

	mockIncidentCounters := map[string][]*pagerduty.IncidentOccurrenceTracker{
		"PD12345": {
			{IncidentName: "Network Outage", Count: 3, LastOccurrence: "2024-02-22"},
			{IncidentName: "Service Downtime", Count: 2, LastOccurrence: "2024-02-20"},
		},
	}
	mockServiceIDs := []string{"PD12345"}
	mockSinceDays := 7

	output := captureOutput(func(w io.Writer) {
		printHistoricalPDAlertSummary(mockIncidentCounters, mockServiceIDs, mockSinceDays, w)

	})

	expectedStrings := []string{
		"PagerDuty Historical Alerts",
		"Service: https://redhat.pagerduty.com/service-directory/PD12345:",
		"Type", "Count", "Last Occurrence",
		"Network Outage", "3", "2024-02-22",
		"Service Downtime", "2", "2024-02-20",
		"Total number of incidents [ 5 ] in [ 7 ] days",
	}

	for _, expected := range expectedStrings {
		assert.Contains(t, output, expected, "Output should contain expected text: %s", expected)
	}
}

func captureOutput(f func(io.Writer)) string {
	var buf bytes.Buffer
	f(&buf) // Passing the buffer as an io.Writer
	return buf.String()
}

func TestPrintShortOutput(t *testing.T) {
	opts := &contextOptions{days: 7}

	limitedSupportReason, _ := v1.NewLimitedSupportReason().Build()
	serviceLog1, _ := v2.NewLogEntry().
		Description("Log 1").
		Timestamp(time.Now()).
		Build()

	serviceLog2, _ := v2.NewLogEntry().
		Description("Log 2").
		Timestamp(time.Now()).
		Build()

	jiraIssue := jira.Issue{Key: "JIRA-300"}
	pdAlert1 := pd.Incident{IncidentKey: "PD-ALERT-2", Urgency: "high"}
	pdAlert2 := pd.Incident{IncidentKey: "PD-ALERT-3", Urgency: "low"}
	historicalAlert := &pagerduty.IncidentOccurrenceTracker{
		Count: 5,
	}

	data := &contextData{
		ClusterName:           "short-cluster",
		ClusterVersion:        "4.11",
		LimitedSupportReasons: []*v1.LimitedSupportReason{limitedSupportReason},
		ServiceLogs:           []*v2.LogEntry{serviceLog1, serviceLog2},
		JiraIssues:            []jira.Issue{jiraIssue},
		PdAlerts:              map[string][]pd.Incident{"service-2": {pdAlert1, pdAlert1, pdAlert2}},
		HistoricalAlerts:      map[string][]*pagerduty.IncidentOccurrenceTracker{"service-2": {historicalAlert}},
	}

	output := captureOutput(func(w io.Writer) {
		opts.printShortOutput(data, w)
	})

	assert.Contains(t, output, "Version")
	assert.Contains(t, output, "Supported?")
	assert.Contains(t, output, "SLs (last 7 d)")
	assert.Contains(t, output, "Jira Tickets")
	assert.Contains(t, output, "Current Alerts")
	assert.Contains(t, output, "Historical Alerts (last 7 d)")
	assert.Contains(t, output, "H: 2 | L: 1")
}

func TestPrintJsonOutput(t *testing.T) {
	opts := &contextOptions{}
	jiraIssue := jira.Issue{Key: "JIRA-999"}

	data := &contextData{
		Description:    "JSON Test Cluster",
		ClusterVersion: "4.9",
		JiraIssues:     []jira.Issue{jiraIssue},
	}

	output := captureOutput(func(w io.Writer) {
		opts.printJsonOutput(data, w)
	})

	var result map[string]interface{}
	err := json.Unmarshal([]byte(output), &result)
	assert.NoError(t, err)
	assert.Contains(t, output, `"JSON Test Cluster"`)
	assert.Contains(t, output, `"4.9"`)
	assert.Contains(t, output, `"JIRA-999"`)
}

func TestPrintLongOutput(t *testing.T) {

	serviceLog1, _ := v2.NewLogEntry().
		Description("Log 1").
		Timestamp(time.Now()).
		Build()

	serviceLog2, _ := v2.NewLogEntry().
		Description("Log 2").
		Timestamp(time.Now()).
		Build()

	limitedSupportReason1, _ := v1.NewLimitedSupportReason().
		Details("Limited Support Reason 1").
		Build()

	eventTime := time.Now()

	mockData := &contextData{
		ClusterName:     "ClusterABC",
		ClusterVersion:  "1.2.3",
		ClusterID:       "cluster-123",
		OCMEnv:          "production",
		DyntraceEnvURL:  "http://dynatrace.example.com",
		DyntraceLogsURL: "http://logs.dynatrace.example.com",
		LimitedSupportReasons: []*v1.LimitedSupportReason{
			limitedSupportReason1},
		ServiceLogs: []*v2.LogEntry{serviceLog1, serviceLog2},
		JiraIssues: []jira.Issue{
			{
				Key: "JIRA-123",
				ID:  "Issue Summary 1",
				Fields: &jira.IssueFields{
					Type: jira.IssueType{
						Name: "Bug",
					},
					Priority: &jira.Priority{
						Name: "High",
					},
					Summary: "Mocked Issue Summary",
					Status: &jira.Status{
						Name: "Open",
					},
				},
			},
		},
		SupportExceptions: []jira.Issue{
			{Key: "JIRA-456", ID: "Exception Summary 1", Fields: &jira.IssueFields{
				Type: jira.IssueType{
					Name: "Bug2",
				},
				Priority: &jira.Priority{
					Name: "Medium",
				},
				Summary: "Mocked Issue Summary2",
				Status: &jira.Status{
					Name: "Open",
				},
			}},
		},
		PdAlerts: map[string][]pd.Incident{
			"Service1": {pd.Incident{Title: "incident-1"}},
		},
		HistoricalAlerts: map[string][]*pagerduty.IncidentOccurrenceTracker{
			"Service1": {&pagerduty.IncidentOccurrenceTracker{IncidentName: "tracker-1"}},
		},
		CloudtrailEvents: []*types.Event{
			{
				EventId:   new(string),
				EventName: new(string),
				Username:  new(string),
				EventTime: &eventTime,
			},
		},
		Description: "This is the cluster description.",
	}

	*mockData.CloudtrailEvents[0].EventName = "Event1"
	*mockData.CloudtrailEvents[0].EventId = "evt-1234567890"
	*mockData.CloudtrailEvents[0].Username = "mockUser"

	o := &contextOptions{
		verbose: true,
		days:    30,
		full:    true,
	}

	output := captureOutput(func(w io.Writer) {
		o.printLongOutput(mockData, w)
	})

	assert.Contains(t, output, "ClusterABC")
	assert.Contains(t, output, "cluster-123")
	assert.Contains(t, output, "Event1")
	assert.Contains(t, output, "mockUser")
	assert.Contains(t, output, "This is the cluster description.")

}

func TestRun_UnknownOutput(t *testing.T) {
	contextOptions := &contextOptions{
		output: "invalidOutputFormat",
	}

	err := contextOptions.run()

	if err == nil || err.Error() != "unknown Output Format: invalidOutputFormat" {
		t.Errorf("Expected unknown output format error, got: %v", err)
	}
}

func TestPrintUserBannedStatus(t *testing.T) {
	tests := []struct {
		name           string
		data           contextData
		expectedOutput string
	}{
		{
			name: "User is banned due to export control compliance",
			data: contextData{
				UserBanned:     true,
				BanCode:        BanCodeExportControlCompliance,
				BanDescription: "Banned for compliance reasons",
			},
			expectedOutput: "\n>> User Ban Details\nUser is banned\nBan code = export_control_compliance\nBan description = Banned for compliance reasons\nUser banned due to export control compliance.\nPlease follow the steps detailed here: https://github.com/openshift/ops-sop/blob/master/v4/alerts/UpgradeConfigSyncFailureOver4HrSRE.md#user-banneddisabled-due-to-export-control-compliance .\n",
		},
		{
			name: "User is banned but not due to export control compliance",
			data: contextData{
				UserBanned:     true,
				BanCode:        "SomeOtherBanCode",
				BanDescription: "Some other reason",
			},
			expectedOutput: "\n>> User Ban Details\nUser is banned\nBan code = SomeOtherBanCode\nBan description = Some other reason\n",
		},
		{
			name: "User is not banned",
			data: contextData{
				UserBanned:     false,
				BanCode:        "",
				BanDescription: "",
			},
			expectedOutput: "\n>> User Ban Details\nUser is not banned\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualOutput := captureOutput(func(w io.Writer) {
				printUserBannedStatus(&tt.data, w)
			})

			expected := strings.TrimSpace(tt.expectedOutput)
			actual := strings.TrimSpace(actualOutput)

			if expected != actual {
				t.Errorf("expected:\n%q\ngot:\n%q", expected, actual)
			}
		})
	}
}
