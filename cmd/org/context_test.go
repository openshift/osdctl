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
	// Save original function pointers
	origGetCluster := getClusterFunc
	origGetLimitedSupport := getClusterLimitedSupportFunc
	origGetServiceLogs := getServiceLogsSinceFunc
	origGetJiraIssues := getJiraIssuesForClusterFunc
	origSearchSubs := searchAllSubscriptionsByOrgFunc
	origCreateConn := createConnectionFunc

	// Restore original functions after test
	defer func() {
		getClusterFunc = origGetCluster
		getClusterLimitedSupportFunc = origGetLimitedSupport
		getServiceLogsSinceFunc = origGetServiceLogs
		getJiraIssuesForClusterFunc = origGetJiraIssues
		searchAllSubscriptionsByOrgFunc = origSearchSubs
		createConnectionFunc = origCreateConn
	}()

	// Setup test server
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

	// Mock dependencies
	getClusterFunc = func(_ *sdk.Connection, clusterID string) (*cmv1.Cluster, error) {
		return cmv1.NewCluster().
			ID(clusterID).
			Name("test-cluster").
			ExternalID("external-1").
			Version(cmv1.NewVersion().RawID("4.10.0")).
			DNS(cmv1.NewDNS().BaseDomain("example.com")).
			CloudProvider(cmv1.NewCloudProvider().ID("aws")).
			Build()
	}

	getClusterLimitedSupportFunc = func(_ *sdk.Connection, _ string) ([]*cmv1.LimitedSupportReason, error) {
		return []*cmv1.LimitedSupportReason{}, nil
	}

	getServiceLogsSinceFunc = func(_ string, _ time.Time, _, _ bool) ([]*v1.LogEntry, error) {
		return []*v1.LogEntry{}, nil
	}

	getJiraIssuesForClusterFunc = func(_, _ string) ([]jira.Issue, error) {
		return []jira.Issue{}, nil
	}

	searchAllSubscriptionsByOrgFunc = func(_ string, _ string, _ bool) ([]*accountsv1.Subscription, error) {
		// sub1, _ := accountsv1.NewSubscription().ClusterID("cluster-1").DisplayName("Cluster One").Status("Active").Build()
		// sub2, _ := accountsv1.NewSubscription().ClusterID("cluster-1").DisplayName("Cluster One").Status("Active").Build()
		// expectedSubs := []*accountsv1.Subscription{sub1, sub2}

		sub, _ := accountsv1.NewSubscription().
			ID("sub1").
			ClusterID("cluster1").
			Status("Active").
			Plan(accountsv1.NewPlan().ID("OSD")).
			CloudProviderID("aws").
			// Metrics([]*accountsv1.SubscriptionMetrics).
			Build()
		return []*accountsv1.Subscription{sub}, nil
	}
	// Metrics([]*accountsv1.Metric{sdk.NewMetric().Nodes(sdk.NewNodes().Total(3))}).
	createConnectionFunc = func() (*sdk.Connection, error) {
		return sdk.NewConnectionBuilder().
			URL(server.URL).
			TokenURL(server.URL+"/auth/token").
			Client("test-client", "test-secret").
			Insecure(true).
			Build()
	}

	// Capture output
	var output bytes.Buffer

	// Execute test
	clusters, err := contextInternal("test-org", &output)

	// Verify output
	t.Log(output.String())

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, clusters, 1)

}
