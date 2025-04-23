package org

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

func getUserTestData() (string, error) {
	data := map[string]interface{}{
		"created_at": "2024-11-27T07:32:59.368563Z",
		"email":      "kilgore.trout@redhat.com",
		"first_name": "kilgore",
		"href":       "/api/accounts_mgmt/v1/accounts/xyz",
		"id":         "foobar",
		"kind":       "Account",
		"last_name":  "Trout",
		"organization": map[string]interface{}{
			"created_at":     "2019-02-15T20:26:12.542449Z",
			"ebs_account_id": "1111111",
			"external_id":    "2222222",
			"href":           "/api/accounts_mgmt/v1/organizations/xyz",
			"id":             "abc.xyz",
			"kind":           "Organization",
			"name":           "Kurt Vonnegut Appreciation Society",
			"updated_at":     "2025-03-10T06:16:08.047253Z",
		},
		"rhit_web_user_id": "57712380",
		"updated_at":       "2025-02-21T21:05:50.544761Z",
		"username":         "kilgore.trout",
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal test data: %w", err)
	}
	return string(jsonData), nil
}

func TestGetCurrentOrg(t *testing.T) {
	userData, err := getUserTestData()
	if err != nil {
		t.Errorf("invalid user test data")
	}
	orgs, err := getCurrentOrg([]byte(userData))
	if err != nil {
		t.Fatal(err)
	}
	name := "Kurt Vonnegut Appreciation Society"
	if orgs.Name != name {
		t.Errorf("Expected %s to equal %s", orgs.Name, name)
	}
	id := "abc.xyz"
	if orgs.ID != id {
		t.Errorf("Expected %s to equal %s", orgs.ID, id)
	}
}

func TestGetOrgRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.URL.Path == tokenPath {
			w.Write([]byte(`{"access_token": "test-token"}`))
			return
		}
		userData, err := getUserTestData()
		if err != nil {
			t.Errorf("invalid user test data")
		}
		w.Write([]byte(userData))
	}))
	defer server.Close()

	ocmClient, err := sdk.NewConnectionBuilder().
		URL(server.URL).
		TokenURL(server.URL+tokenPath).
		Insecure(true).
		Client(clientID, clientSecret).
		Build()
	if err != nil {
		t.Fatalf("Failed to build connection: %v", err)
	}
	defer ocmClient.Close()

	req, err := getOrgRequest(ocmClient)
	if err != nil {
		t.Fatal(err)
	}
	if req.GetPath() != currentAccountApiPath {
		t.Errorf("%s does not equal %s", req.GetPath(), currentAccountApiPath)
	}
}
