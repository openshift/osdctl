package org

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/openshift/osdctl/pkg/utils"
)

var sampleCustomersJSON = `
{
	"items": [
		{"id": "cust1", "cluster_count": 3},
		{"id": "cust2", "cluster_count": 5}
	]
}
`

func TestParseCustomers(t *testing.T) {
	customers, err := parseCustomers([]byte(sampleCustomersJSON))
	if err != nil {
		t.Fatalf("Failed to parse customers: %v", err)
	}

	if len(customers) != 2 {
		t.Errorf("Expected 2 customers, got %d", len(customers))
	}

	expected := []Customer{
		{ID: "cust1", ClusterCount: 3},
		{ID: "cust2", ClusterCount: 5},
	}

	for i, customer := range customers {
		if customer.ID != expected[i].ID {
			t.Errorf("Customer %d: Expected ID %s, got %s", i, expected[i].ID, customer.ID)
		}
		if customer.ClusterCount != expected[i].ClusterCount {
			t.Errorf("Customer %d: Expected ClusterCount %d, got %d", i, expected[i].ClusterCount, customer.ClusterCount)
		}
	}
}

func TestGetCustomersRequest(t *testing.T) {
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		t.Fatalf("Failed to create OCM connection: %v", err)
	}
	defer ocmClient.Close()

	orgID := "test-org-id"
	req, err := getCustomersRequest(ocmClient, orgID)
	if err != nil {
		t.Fatalf("Failed to build customers request: %v", err)
	}

	expectedPath := "/api/accounts_mgmt/v1/organizations/test-org-id/customers"
	if req.GetPath() != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, req.GetPath())
	}
}

func TestPrintCustomers(t *testing.T) {
	var response struct {
		Items []Customer `json:"items"`
	}
	err := json.Unmarshal([]byte(sampleCustomersJSON), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal sample JSON: %v", err)
	}

	expected := "Customer ID: cust1, Cluster Count: 3\nCustomer ID: cust2, Cluster Count: 5\n"

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printCustomers(response.Items)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if output != expected {
		t.Errorf("Expected:\n%sGot:\n%s", expected, output)
	}
}
