package org

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

func Test_getCustomers(t *testing.T) {
	t.Run("Success Test", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if r.URL.Path == "/oauth2/token" {
				w.Write([]byte(`{"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXIiLCJleHAiOjI1MjQ2MDgwMDB9.signature-placeholder", "token_type": "Bearer", "expires_in": 3600}`))
				return
			}
			response := map[string]interface{}{
				"page":  1,
				"size":  2,
				"total": 2,
				"items": []map[string]string{
					{"kind": "ResourceQuota", "id": "cust-1", "organization_id": "org-1", "sku": "sku-1"},
					{"kind": "ResourceQuota", "id": "cust-2", "organization_id": "org-2", "sku": "sku-2"},
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		conn, err := sdk.NewConnectionBuilder().
			URL(server.URL).
			TokenURL(server.URL + "/oauth2/token").
			Insecure(true).
			Client("fake-id", "fake-secret").
			Build()
		if err != nil {
			t.Fatalf("Failed to build connection: %v", err)
		}

		customers, err := getCustomers(nil, conn)
		if err != nil {
			t.Errorf("getCustomers() returned an error: %v", err)
		}

		expected := []Customer{
			{ID: "cust-1", OrganizationID: "org-1", SKU: "sku-1"},
			{ID: "cust-2", OrganizationID: "org-2", SKU: "sku-2"},
		}

		if len(customers) != len(expected) {
			t.Errorf("Expected %d customers, got %d", len(expected), len(customers))
		}

		for i, customer := range customers {
			if customer != expected[i] {
				t.Errorf("Expected customer %+v, got %+v", expected[i], customer)
			}
		}
	})
}

