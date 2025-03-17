package org

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

var (
	testToken     = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXIiLCJleHAiOjI1MjQ2MDgwMDB9.signature-placeholder"
	clientID      = "fake-id"
	clientSecret  = "fake-secret"
	tokenPath     = "/oauth2/token"
	testCustomers = []Customer{
		{ID: "cust-1", OrganizationID: "org-1", SKU: "sku-1"},
		{ID: "cust-2", OrganizationID: "org-2", SKU: "sku-2"},
	}
)

func Test_getCustomers(t *testing.T) {
	apiResponse := map[string]interface{}{
		"page":  1,
		"size":  len(testCustomers),
		"total": len(testCustomers),
		"items": []map[string]string{
			{"kind": "ResourceQuota", "id": "cust-1", "organization_id": "org-1", "sku": "sku-1"},
			{"kind": "ResourceQuota", "id": "cust-2", "organization_id": "org-2", "sku": "sku-2"},
		},
	}

	tokenResponse := map[string]interface{}{
		"access_token": testToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
	}

	t.Run("Success Test", func(t *testing.T) {
		// Setup test server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			// Handle token request
			if r.URL.Path == tokenPath {
				json.NewEncoder(w).Encode(tokenResponse)
				return
			}

			// Handle API request
			json.NewEncoder(w).Encode(apiResponse)
		}))
		defer server.Close()

		// Create connection
		conn, err := sdk.NewConnectionBuilder().
			URL(server.URL).
			TokenURL(server.URL+tokenPath).
			Insecure(true).
			Client(clientID, clientSecret).
			Build()
		if err != nil {
			t.Fatalf("Failed to build connection: %v", err)
		}

		customers, err := getCustomers(nil, conn)

		if err != nil {
			t.Fatalf("getCustomers() returned an error: %v", err)
		}

		if !reflect.DeepEqual(customers, testCustomers) {
			t.Errorf("Expected customers %+v, got %+v", testCustomers, customers)
		}
	})
}

func TestPrintCustomers_TableOutput(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printCustomers(testCustomers)

	// Capture output and restore stdout
	w.Close()
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	if err != nil {
		t.Fatal("Failed to read from stdout pipe:", err)
	}
	os.Stdout = oldStdout

	output := buf.String()
	if !containsAll(output, "ID", "OrganizationID", "SKU", "cust-1", "org-1", "sku-1") {
		t.Errorf("Expected output to contain customer data. Got:\n%s", output)
	}
}

// Helper to check if all substrings exist in a string
func containsAll(str string, substrs ...string) bool {
	for _, s := range substrs {
		if !bytes.Contains([]byte(str), []byte(s)) {
			return false
		}
	}
	return true
}
