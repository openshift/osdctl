package org

import (
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
