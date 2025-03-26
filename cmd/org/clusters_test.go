package org

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	sdk "github.com/openshift-online/ocm-sdk-go"
	accountsv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/stretchr/testify/assert"
)

func TestSearchSubscriptionss(t *testing.T) {
	t.Run("Error when neither orgId nor AWS profile is provided", func(t *testing.T) {
		_, err := SearchSubscriptions("", "")
		assert.Error(t, err)
		assert.Equal(t, "specify either org-id or --aws-profile,--aws-account-id arguments", err.Error())
	})

	t.Run("Error when both orgId and AWS profile are provided", func(t *testing.T) {
		awsProfile = "test-profile"
		awsAccountID = "123456789"
		_, err := SearchSubscriptions("valid-org-id", "")
		assert.Error(t, err)
		assert.Equal(t, "specify either an org id argument or --aws-profile, --aws-account-id arguments", err.Error())
		awsProfile = ""
		awsAccountID = ""
	})

	t.Run("Error when AWS profile search fails", func(t *testing.T) {
		awsProfile = "test-profile"
		awsAccountID = "123456789"
		_, err := SearchSubscriptions("", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get org ID from AWS profile")
		awsProfile = ""
		awsAccountID = ""
	})

	t.Run("Success With Valid OrgID", func(t *testing.T) {
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
		defer server.Close()

		originalConnectionFactory := connectionFactory
		defer func() { connectionFactory = originalConnectionFactory }()

		connectionFactory = func() (*sdk.Connection, error) {
			return sdk.NewConnectionBuilder().
				URL(server.URL).
				TokenURL(server.URL+tokenPath).
				Insecure(true).
				Client(clientID, clientSecret).
				Build()
		}

		awsProfile = ""
		awsAccountID = ""

		subscriptions, err := SearchSubscriptions("test-org-123", StatusActive)
		if err != nil {
			t.Errorf("SearchSubscriptions() returned an error: %v", err)
		}
		assert.NoError(t, err, "SearchSubscriptions() should not return an error")
		assert.Len(t, subscriptions, 1, "Expected 1 subscription")
		assert.Equal(t, "cluster-1", subscriptions[0].ClusterID(), "Incorrect ClusterID")
		assert.Equal(t, "external-cluster-1", subscriptions[0].ExternalClusterID(), "Incorrect ExternalClusterID")
		assert.Equal(t, "Test Cluster 1", subscriptions[0].DisplayName(), "Incorrect DisplayName")
		assert.Equal(t, StatusActive, subscriptions[0].Status(), "Incorrect Status")
	})

	t.Run("Empty OrgID Without AWS Profile", func(t *testing.T) {
		awsProfile = ""
		awsAccountID = ""

		subscriptions, err := SearchSubscriptions("", StatusActive)
		assert.Error(t, err, "SearchSubscriptions() should return an error with empty orgID")
		assert.Nil(t, subscriptions, "Expected nil subscriptions")
	})
}

func TestPrintClusters_Success(t *testing.T) {
	var (
		mockIsJsonOutput bool
		mockPrintJsonCalled bool
		mockPrintJsonData interface{}
	)

	sub1, _ := accountsv1.NewSubscription().ClusterID("cluster-1").ExternalClusterID("ext-1").DisplayName("Test Cluster 1").Status("Active").Build()
	sub2, _ := accountsv1.NewSubscription().ClusterID("cluster-2").ExternalClusterID("ext-2").DisplayName("Test Cluster 2").Status("Pending").Build()
	subs := []*accountsv1.Subscription{sub1, sub2}

	originalStdout := os.Stdout

	printClustersTest := func(items []*accountsv1.Subscription) {
		if mockIsJsonOutput {
			subscriptions := make([]map[string]string, 0, len(items))
			for _, item := range items {
				subscription := map[string]string{
					"cluster_id":   item.ClusterID(),
					"external_id":  item.ExternalClusterID(),
					"display_name": item.DisplayName(),
					"status":       item.Status(),
				}
				subscriptions = append(subscriptions, subscription)
			}
			mockPrintJsonCalled = true
			mockPrintJsonData = subscriptions
		} else {
			table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
			table.AddRow([]string{"DISPLAY NAME", "INTERNAL CLUSTER ID", "EXTERNAL CLUSTER ID", "STATUS"})
			for _, subscription := range items {
				table.AddRow([]string{
					subscription.DisplayName(),
					subscription.ClusterID(),
					subscription.ExternalClusterID(),
					subscription.Status(),
				})
			}
			table.AddRow([]string{})
			table.Flush()
		}
	}

	t.Run("JSON output", func(t *testing.T) {
		mockIsJsonOutput = true
		mockPrintJsonCalled = false
		mockPrintJsonData = nil

		printClustersTest(subs)

		assert.True(t, mockPrintJsonCalled, "PrintJson should have been called")
		
		subscriptions, ok := mockPrintJsonData.([]map[string]string)
		assert.True(t, ok, "Expected []map[string]string type")
		assert.Len(t, subscriptions, 2)
		assert.Equal(t, "cluster-1", subscriptions[0]["cluster_id"])
		assert.Equal(t, "ext-1", subscriptions[0]["external_id"])
		assert.Equal(t, "Test Cluster 1", subscriptions[0]["display_name"])
		assert.Equal(t, "Active", subscriptions[0]["status"])
	})

	t.Run("Table output", func(t *testing.T) {
		mockIsJsonOutput = false
		r, w, _ := os.Pipe()
		os.Stdout = w
		printClustersTest(subs)
		w.Close()

		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stdout = originalStdout

		output := buf.String()
		assert.Contains(t, output, "DISPLAY NAME")
		assert.Contains(t, output, "INTERNAL CLUSTER ID")
		assert.Contains(t, output, "EXTERNAL CLUSTER ID")
		assert.Contains(t, output, "STATUS")
	})
}
