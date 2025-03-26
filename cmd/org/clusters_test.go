package org

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	sdk "github.com/openshift-online/ocm-sdk-go"
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
