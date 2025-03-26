package org

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/andygrunwald/go-jira"
	"github.com/golang-jwt/jwt/v5"
	sdk "github.com/openshift-online/ocm-sdk-go"
	accountsv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	v1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	"github.com/stretchr/testify/assert"
)

func TestPrintContextJson(t *testing.T) {
	tests := []struct {
		name          string
		clusterInfos  []ClusterInfo
		expectedJSON  string
		expectedError error
	}{
		{
			name: "Valid cluster info",
			clusterInfos: []ClusterInfo{
				{
					Name:          "test-cluster",
					Version:       "4.10.0",
					ID:            "12345",
					CloudProvider: "AWS",
					Plan:          "MOA",
					NodeCount:     3,
					ServiceLogs:   []*v1.LogEntry{},
					PdAlerts:      map[string][]pd.Incident{},
					JiraIssues:    []jira.Issue{},
				},
			},
			expectedJSON: `[
  {
    "displayName": "test-cluster",
    "clusterId": "12345",
    "version": "4.10.0",
    "status": "Fully Supported",
    "provider": "AWS",
    "plan": "ROSA",
    "nodeCount": 3,
    "recentSLs": 0,
    "activePDs": 0,
    "ohssTickets": 0
  }
]`,
			expectedError: nil,
		},
		{
			name:          "Empty cluster info",
			clusterInfos:  []ClusterInfo{},
			expectedJSON:  "[]\n", // JSON output includes a newline from Fprintln
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer

			err := printContextJson(&output, tt.clusterInfos) // Use the updated function

			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
				assert.JSONEq(t, tt.expectedJSON, output.String())
			}
		})
	}
}

func Test_printContext(t *testing.T) {
	type args struct {
		clusterInfos []ClusterInfo
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Valid cluster info",
			args: args{
				clusterInfos: []ClusterInfo{
					{
						Name:          "test-cluster",
						Version:       "4.10.0",
						ID:            "12345",
						CloudProvider: "AWS",
						Plan:          "MOA",
						NodeCount:     3,
						ServiceLogs:   []*v1.LogEntry{},
						PdAlerts:      map[string][]pd.Incident{},
						JiraIssues:    []jira.Issue{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Empty cluster info",
			args: args{
				clusterInfos: []ClusterInfo{},
			},
			wantErr: false,
		},
		{
			name: "Cluster info with limited support",
			args: args{
				clusterInfos: []ClusterInfo{
					{
						Name:          "limited-support-cluster",
						Version:       "4.9.0",
						ID:            "67890",
						CloudProvider: "GCP",
						Plan:          "MOA-HostedControlPlane",
						NodeCount:     5,
						ServiceLogs:   []*v1.LogEntry{},
						JiraIssues: []jira.Issue{
							{
								Key: "OCM-123",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Cluster info with service logs and PD alerts",
			args: args{
				clusterInfos: []ClusterInfo{
					{
						Name:          "cluster-with-logs",
						Version:       "4.8.0",
						ID:            "54321",
						CloudProvider: "Azure",
						Plan:          "MOA",
						NodeCount:     10,
						ServiceLogs:   []*v1.LogEntry{
							// {
							// 	Severity: "Info",
							// },
						},
						PdAlerts: map[string][]pd.Incident{
							"service1": {
								// {
								// 	Id: "PD-001",
								// },
							},
						},
						JiraIssues: []jira.Issue{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Cluster info with invalid data",
			args: args{
				clusterInfos: []ClusterInfo{
					{
						Name:          "",
						Version:       "",
						ID:            "",
						CloudProvider: "",
						Plan:          "",
						NodeCount:     0,
						ServiceLogs:   nil,
						PdAlerts:      nil,
						JiraIssues:    nil,
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := printContext(tt.args.clusterInfos); (err != nil) != tt.wantErr {
				t.Errorf("printContext() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContext_Success(t *testing.T) {
	// Save original function pointers to restore after test
	origGetCluster := getClusterFunc
	origGetLimitedSupport := getClusterLimitedSupportFunc
	origGetServiceLogs := getServiceLogsSinceFunc
	origGetJiraIssues := getJiraIssuesForClusterFunc
	origSearchSubs := searchAllSubscriptionsByOrgFunc
	origCreateConn := createConnectionFunc
	origAddPDAlerts := addPDAlertsFunc
	origAddLimitedSupportReasons := addLimitedSupportReasonsFunc
	origAddServiceLogs := addServiceLogsFunc
	origAddJiraIssues := addJiraIssuesFunc

	// Restore original functions after test
	defer func() {
		getClusterFunc = origGetCluster
		getClusterLimitedSupportFunc = origGetLimitedSupport
		getServiceLogsSinceFunc = origGetServiceLogs
		getJiraIssuesForClusterFunc = origGetJiraIssues
		searchAllSubscriptionsByOrgFunc = origSearchSubs
		createConnectionFunc = origCreateConn
		addPDAlertsFunc = origAddPDAlerts
		addLimitedSupportReasonsFunc = origAddLimitedSupportReasons
		addServiceLogsFunc = origAddServiceLogs
		addJiraIssuesFunc = origAddJiraIssues
	}()

	testToken, _ := jwt.New(jwt.SigningMethodHS256).SignedString([]byte("test-secret"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/token":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": testToken,
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/api/accounts_mgmt/v1/subscriptions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{{
					"id":                "sub1",
					"cluster_id":        "cluster1",
					"status":            "Active",
					"plan":              map[string]interface{}{"id": "OSD"},
					"metrics":           []map[string]interface{}{{"nodes": map[string]interface{}{"total": 3}}},
					"cloud_provider_id": "aws",
				}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	mockCluster, _ := cmv1.NewCluster().
		ID("cluster1").
		Name("test-cluster").
		ExternalID("external-1").
		Version(cmv1.NewVersion().RawID("4.10.0")).
		DNS(cmv1.NewDNS().BaseDomain("example.com")).
		CloudProvider(cmv1.NewCloudProvider().ID("aws")).
		Build()

	getClusterFunc = func(_ *sdk.Connection, clusterID string) (*cmv1.Cluster, error) {
		return mockCluster, nil
	}

	mockLimitedSupportReasons := []*cmv1.LimitedSupportReason{}

	getClusterLimitedSupportFunc = func(_ *sdk.Connection, _ string) ([]*cmv1.LimitedSupportReason, error) {
		return mockLimitedSupportReasons, nil
	}

	mockServiceLogs := []*v1.LogEntry{}

	getServiceLogsSinceFunc = func(_ string, _ time.Time, _, _ bool) ([]*v1.LogEntry, error) {
		return mockServiceLogs, nil
	}

	mockJiraIssues := []jira.Issue{}

	getJiraIssuesForClusterFunc = func(_, _ string) ([]jira.Issue, error) {
		return mockJiraIssues, nil
	}

	mockSubscription, _ := accountsv1.NewSubscription().
		ID("sub1").
		ClusterID("cluster1").
		Status("Active").
		Plan(accountsv1.NewPlan().ID("OSD")).
		CloudProviderID("aws").
		Build()

	searchAllSubscriptionsByOrgFunc = func(_ string, _ string, _ bool) ([]*accountsv1.Subscription, error) {
		return []*accountsv1.Subscription{mockSubscription}, nil
	}

	createConnectionFunc = func() (*sdk.Connection, error) {
		return sdk.NewConnectionBuilder().
			URL(server.URL).
			TokenURL(server.URL+"/auth/token").
			Client("test-client", "test-secret").
			Insecure(true).
			Build()
	}

	addLimitedSupportReasonsFunc = func(ci *ClusterInfo, _ *sdk.Connection) error {
		ci.LimitedSupportReasons = mockLimitedSupportReasons
		return nil
	}

	addServiceLogsFunc = func(ci *ClusterInfo) error {
		ci.ServiceLogs = mockServiceLogs
		return nil
	}

	addJiraIssuesFunc = func(ci *ClusterInfo, _ string) error {
		ci.JiraIssues = mockJiraIssues
		return nil
	}

	mockPDAlerts := map[string][]pd.Incident{}

	addPDAlertsFunc = func(ci *ClusterInfo, _ string) error {
		ci.PdAlerts = mockPDAlerts
		return nil
	}

	var output bytes.Buffer
	clusters, err := contextInternal("test-org", &output)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(clusters))
	assert.Equal(t, "test-cluster", clusters[0].Name)
	assert.Equal(t, "cluster1", clusters[0].ID)
	assert.Equal(t, "4.10.0", clusters[0].Version)
	assert.Equal(t, "aws", clusters[0].CloudProvider)
	assert.Equal(t, "OSD", clusters[0].Plan)

	outputStr := output.String()
	assert.Contains(t, outputStr, "Fetching data for 1 clusters in org test-org")
	assert.Contains(t, outputStr, "Fetched data for 1 of 1 clusters")
}
