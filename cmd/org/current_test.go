package org

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

func Test_run(t *testing.T) {

	t.Run("Success Test", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if r.URL.Path == "/oauth2/token" {
				w.Write([]byte(`{"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXIiLCJleHAiOjI1MjQ2MDgwMDB9.signature-placeholder", "token_type": "Bearer", "expires_in": 3600}`))
				return
			}
			response := map[string]interface{}{
				"organization": map[string]interface{}{
					"id":             "1",
					"external_id":    "external_id",
					"name":           "name",
					"ebs_account_id": "ebs_account_id",
					"created_at":     "created_at",
					"updated_at":     "updated_at",
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		conn, err := sdk.NewConnectionBuilder().
			URL(server.URL).
			TokenURL(server.URL+"/oauth2/token").
			Insecure(true).
			Client("fake-id", "fake-secret").
			Build()
		if err != nil {
			t.Fatalf("Failed to build connection: %v", err)
		}
		err = run(nil, conn)
		if err != nil {
			t.Errorf("getCustomers() returned an error: %v", err)
		}
	})
}
