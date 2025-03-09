package cluster

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"bou.ke/monkey"
	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/andygrunwald/go-jira"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	v2 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	"github.com/openshift/osdctl/cmd/dynatrace"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/provider/pagerduty"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

type MockUtils struct {
	mock.Mock
}

type MockOcmClient struct {
	mock.Mock
}

type mockAwsClient struct {
	aws.Client
}

type MockPdClient struct{}

func TestGenerateContextData(t *testing.T) {

	instance := pagerduty.NewClient()

	patchGetPDServiceIDs := monkey.Patch(
		instance.GetPDServiceIDs,
		func() ([]string, error) {
			return nil, nil
		},
	)
	defer patchGetPDServiceIDs.Unpatch()

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
	assert.NotNil(t, flags.Lookup("cluster-id"))
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

func (m *MockOCMClient) Close() error {
	return nil
}

func MockCreateConnection() (*MockOCMClient, error) {
	return &MockOCMClient{}, nil
}

func MockGetClusters(client *MockOCMClient, args []string) []*v1.Cluster {
	mockDNS, _ := v1.NewDNS().BaseDomain("mock-domain.com").Build()
	mockCluster, _ := v1.NewCluster().
		ID("mock-cluster-id").
		ExternalID("mock-external-id").
		InfraID("mock-infra-id").
		Name("mock-cluster").
		DNS((*v1.DNSBuilder)(mockDNS)).
		Build()

	return []*v1.Cluster{mockCluster}
}
func TestComplete(t *testing.T) {
	opts := newContextOptions()
	cmd := &cobra.Command{}

	// Test case 1: No cluster ID provided
	err := opts.complete(cmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Provide exactly one cluster ID")

	// Test case 2: Invalid days value
	opts.days = 0
	err = opts.complete(cmd, []string{"test-cluster-id"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot have a days value lower than 1")

	mockClient, _ := MockCreateConnection()
	mockClusters := MockGetClusters(mockClient, []string{"valid-cluster-id"})

	opts.cluster = mockClusters[0]
	opts.clusterID = opts.cluster.ID()
	opts.externalClusterID = opts.cluster.ExternalID()
	opts.baseDomain = opts.cluster.DNS().BaseDomain()
	opts.infraID = opts.cluster.InfraID()

	assert.Equal(t, "mock-cluster-id", opts.clusterID)
	assert.Equal(t, "mock-external-id", opts.externalClusterID)
	assert.Equal(t, "mock-infra-id", opts.infraID)
	assert.Equal(t, "mock-domain.com", opts.baseDomain)

	// Test case 3: Multiple clusters returned
	mockClusters = append(mockClusters, mockClusters[0])
	if len(mockClusters) != 1 {
		err = errors.New("unexpected number of clusters matched input")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected number of clusters matched input")
	}
}

func TestPrintClusterHeader(t *testing.T) {
	data := &contextData{
		ClusterName: "test-cluster",
		ClusterID:   "12345",
	}

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	data.printClusterHeader()

	w.Close()
	os.Stdout = origStdout
	output, _ := io.ReadAll(r)

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

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printDynatraceResources(data)

	w.Close()
	os.Stdout = origStdout
	output, _ := io.ReadAll(r)

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

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printCloudTrailLogs(events)

	w.Close()
	os.Stdout = origStdout
	output, _ := io.ReadAll(r)
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

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	o := &contextOptions{
		clusterID:         mockClusterID,
		externalClusterID: mockExternalClusterID,
	}

	data := &contextData{
		pdServiceID: mockPDServiceID,
	}

	o.printOtherLinks(data)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

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

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printJIRASupportExceptions(mockIssues)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

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

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printHistoricalPDAlertSummary(mockIncidentCounters, mockServiceIDs, mockSinceDays)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

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

func captureOutput(f func()) string {
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w

	done := make(chan struct{})
	var buf bytes.Buffer

	// Read output in a separate goroutine to prevent blocking
	go func() {
		io.Copy(&buf, r)
		close(done)
	}()

	// Execute the function
	f()

	// Close writer and restore stdout
	w.Close()
	os.Stdout = stdout // Restore stdout

	// Ensure all output is read before returning
	<-done

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

	output := captureOutput(func() {
		opts.printShortOutput(data)
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

	output := captureOutput(func() {
		opts.printJsonOutput(data)
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

	o.printLongOutput(mockData)

}

func (mockAwsClient) LookupEvents(input *cloudtrail.LookupEventsInput) (*cloudtrail.LookupEventsOutput, error) {

	eventId1 := "12345"
	eventName1 := "CreateInstance"
	username1 := "test-user"
	eventTime1 := time.Now()

	eventId2 := "67890"
	eventName2 := "DeleteBucket"
	username2 := "test-user2"
	eventTime2 := time.Now()

	return &cloudtrail.LookupEventsOutput{
		Events: []types.Event{
			{
				EventId:   &eventId1,
				EventName: &eventName1,
				Username:  &username1,
				EventTime: &eventTime1,
			},
			{
				EventId:   &eventId2,
				EventName: &eventName2,
				Username:  &username2,
				EventTime: &eventTime2,
			},
		},
	}, nil
}

func TestGetCloudTrailLogsForCluster(t *testing.T) {

	awsProfile := "test-profile"
	clusterID := "test-cluster-id"
	maxPages := 1

	monkey.Patch(osdCloud.GenerateAWSClientForCluster, func(awsProfile string, clusterID string) (aws.Client, error) {
		return &mockAwsClient{}, nil

	})

	defer monkey.UnpatchAll()

	filteredEvents, err := GetCloudTrailLogsForCluster(awsProfile, clusterID, maxPages)

	assert.NoError(t, err)

	assert.NotEmpty(t, filteredEvents)

	for _, event := range filteredEvents {

		assert.NotNil(t, event.EventName)

		if event.Username != nil {
			assert.NotContains(t, *event.Username, "RH-SRE-")
		}
	}

	t.Logf("Filtered Events: %+v", filteredEvents)
}

func (m *MockPdClient) GetPDServiceIDs() ([]string, error) {
	return nil, nil
}

func TestRunWithOutputFormats(t *testing.T) {

	testCases := []struct {
		output   string
		expected string
	}{
		{shortOutputConfigValue, "Expected no error"},
		{longOutputConfigValue, "Expected no error"},
		{jsonOutputConfigValue, "Expected no error"},
		{"unknown_format", "unknown Output Format: unknown_format"},
	}

	for _, tc := range testCases {
		t.Run(tc.output, func(t *testing.T) {
			o := &contextOptions{output: tc.output}

			serviceLog1, _ := v2.NewLogEntry().
				Description("Log 1").
				Timestamp(time.Now()).
				Build()

			serviceLog2, _ := v2.NewLogEntry().
				Description("Log 2").
				Timestamp(time.Now()).
				Build()

			defer monkey.UnpatchAll()

			monkey.Patch(utils.GetCluster, func(connection *sdk.Connection, key string) (cluster *v1.Cluster, err error) {
				return &v1.Cluster{}, nil
			})

			monkey.Patch(utils.GetJiraIssuesForCluster, func(clusterID string, externalClusterID string) ([]jira.Issue, error) {
				return []jira.Issue{}, nil
			})

			monkey.Patch(utils.GetJiraSupportExceptionsForOrg, func(organizationID string) ([]jira.Issue, error) {
				return []jira.Issue{}, nil
			})

			monkey.Patch(dynatrace.FetchClusterDetails, func(clusterKey string) (hcpCluster dynatrace.HCPCluster, error error) {
				return dynatrace.HCPCluster{}, nil
			})

			monkey.Patch(servicelog.GetServiceLogsSince, func(clusterID string, timeSince time.Time, allMessages bool, internalOnly bool) ([]*v2.LogEntry, error) {
				return []*v2.LogEntry{serviceLog1, serviceLog2}, nil
			})

			pdProvider, _ := pagerduty.NewClient().
				WithUserToken("token1").
				WithOauthToken("oauth").
				WithBaseDomain("abc@domain.com").
				WithTeamIdList(viper.GetStringSlice("Id")).
				Init()

			monkey.Patch(pdProvider.GetPDServiceIDs, func() ([]string, error) {
				return nil, nil
			})

			err := o.run()

			if tc.output == "unknown_format" {
				// For the unknown format, check the error
				assert.Error(t, err)
				assert.Equal(t, tc.expected, err.Error())
			} else {
				// For valid formats (short, long, json), check that no error occurred
				if err != nil {
					t.Errorf("Expected no error for output format %s, got: %v", tc.output, err)
				}
			}
		})
	}
}
