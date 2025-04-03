package org

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	sdk "github.com/openshift-online/ocm-sdk-go"
	accountsv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/stretchr/testify/assert"
)

func setupMockServer() (*httptest.Server, func()) {
	testToken, _ := jwt.New(jwt.SigningMethodHS256).SignedString([]byte("test-secret"))
	clientID := "fake-id"
	clientSecret := "fake-secret"
	tokenPath := "/auth/token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == tokenPath {
			tokenResponse := map[string]interface{}{
				"access_token": testToken,
				"token_type":   "Bearer",
				"expires_in":   3600,
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(tokenResponse)
			return
		}

		if r.URL.Path == "/api/accounts_mgmt/v1/subscriptions" {
			response := map[string]interface{}{
				"kind":  "SubscriptionList",
				"page":  1,
				"size":  2,
				"total": 2,
				"items": []map[string]interface{}{
					{
						"id":                  "subscription-1",
						"kind":                "Subscription",
						"href":                "/api/accounts_mgmt/v1/subscriptions/subscription-1",
						"status":              "Active",
						"cluster_id":          "cluster-1",
						"external_cluster_id": "external-cluster-1",
						"display_name":        "Test Cluster 1",
						"organization_id":     "test-org-123",
					},
				},
			}
			json.NewEncoder(w).Encode(response)
			return
		}
	}))

	// Save original factory
	originalConnectionFactory := connectionFactory

	// Set the connection factory for this test
	connectionFactory = func() (*sdk.Connection, error) {
		return sdk.NewConnectionBuilder().
			URL(server.URL).
			TokenURL(server.URL+tokenPath).
			Insecure(true).
			Client(clientID, clientSecret).
			Build()
	}

	// Return server and cleanup function that restores original factory
	return server, func() {
		server.Close()
		connectionFactory = originalConnectionFactory
	}
}

func TestSearchSubscriptions(t *testing.T) {
	_, cleanup := setupMockServer()
	defer cleanup()

	tests := []struct {
		name          string
		orgId         string
		status        string
		awsProfile    string
		awsAccountID  string
		expectedError string
		validate      func(t *testing.T, result []*accountsv1.Subscription)
	}{
		{
			name:          "Error when neither orgId nor AWS profile is provided",
			orgId:         "",
			status:        "Active",
			awsProfile:    "",
			awsAccountID:  "",
			expectedError: "specify either org-id or --aws-profile,--aws-account-id arguments",
		},
		{
			name:          "Error when both orgId and AWS profile are provided",
			orgId:         "test-org-id",
			status:        "Active",
			awsProfile:    "test-profile",
			awsAccountID:  "test-account-id",
			expectedError: "specify either an org id argument or --aws-profile, --aws-account-id arguments",
		},
		{
			name:          "Error when AWS profile search fails",
			orgId:         "",
			status:        "Active",
			awsProfile:    "invalid-profile",
			awsAccountID:  "invalid-account-id",
			expectedError: "failed to get org ID from AWS profile",
		},
		{
			name:          "Empty OrgID Without AWS Profile",
			orgId:         "",
			status:        "Active",
			awsProfile:    "",
			awsAccountID:  "",
			expectedError: "specify either org-id or --aws-profile,--aws-account-id arguments",
		},
		{
			name:          "Success With Valid OrgID",
			orgId:         "test-org-123",
			status:        "Active",
			awsProfile:    "",
			awsAccountID:  "",
			expectedError: "",
			validate: func(t *testing.T, result []*accountsv1.Subscription) {
				assert.NotEmpty(t, result, "Result should not be empty")
				if len(result) > 0 {
					assert.Len(t, result, 1, "Expected 1 subscription")
					assert.Equal(t, "cluster-1", result[0].ClusterID(), "Incorrect ClusterID")
					assert.Equal(t, "external-cluster-1", result[0].ExternalClusterID(), "Incorrect ExternalClusterID")
					assert.Equal(t, "Test Cluster 1", result[0].DisplayName(), "Incorrect DisplayName")
					assert.Equal(t, StatusActive, result[0].Status(), "Incorrect Status")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			awsProfile = tt.awsProfile
			awsAccountID = tt.awsAccountID
			result, err := SearchSubscriptions(tt.orgId, tt.status)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestPrintClusters_OutputFormat(t *testing.T) {
	sub1, _ := accountsv1.NewSubscription().
		ClusterID("cluster-1").
		ExternalClusterID("ext-1").
		DisplayName("Test Cluster 1").
		Status("Active").
		Build()

	sub2, _ := accountsv1.NewSubscription().
		ClusterID("cluster-2").
		ExternalClusterID("ext-2").
		DisplayName("Test Cluster 2").
		Status("Pending").
		Build()

	subs := []*accountsv1.Subscription{sub1, sub2}

	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	printClusters(subs)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if strings.HasPrefix(strings.TrimSpace(output), "[") {
		t.Log("Detected JSON output")
		var subscriptions []map[string]string
		err := json.Unmarshal([]byte(output), &subscriptions)
		assert.NoError(t, err, "Output should be valid JSON")
		assert.Len(t, subscriptions, 2, "Should have 2 subscriptions")
		assert.Equal(t, "cluster-1", subscriptions[0]["cluster_id"])
		assert.Equal(t, "ext-1", subscriptions[0]["external_id"])
	} else {
		t.Log("Detected table output")
		assert.Contains(t, output, "DISPLAY NAME")
		assert.Contains(t, output, "INTERNAL CLUSTER ID")
		assert.Contains(t, output, "Test Cluster 1")
		assert.Contains(t, output, "cluster-1")
	}
}
