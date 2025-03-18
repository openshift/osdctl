package org

import (
	"testing"
	"github.com/openshift/osdctl/pkg/utils"
)

var orgDetailsData = `
{
  "created_at": "2019-02-15T20:26:12.542449Z",
  "ebs_account_id": "1111111",
  "external_id": "2222222",
  "href": "/api/accounts_mgmt/v1/organizations/abc.xyz",
  "id": "abc.xyz",
  "kind": "Organization",
  "name": "Kurt Vonnegut Appreciation Society",
  "updated_at": "2025-03-10T06:16:08.047253Z"
}
`

func TestDescribeOrg(t *testing.T) {
	org, err := describeOrg([]byte(orgDetailsData))
	if err != nil {
		t.Fatal(err)
	}
	
	name := "Kurt Vonnegut Appreciation Society"
	if org.Name != name {
		t.Errorf("Expected %s to equal %s", org.Name, name)
	}
	
	id := "abc.xyz"
	if org.ID != id {
		t.Errorf("Expected %s to equal %s", org.ID, id)
	}
}

func TestGetDescribeOrgRequest(t *testing.T) {
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		t.Skip("Skipping test: unable to create OCM connection")
	}
	
	orgID := "abc.xyz"
	expectedPath := organizationsAPIPath + "/" + orgID
	
	req, err := getDescribeOrgRequest(ocmClient, orgID)
	if err != nil {
		t.Fatal(err)
	}
	
	if req.GetPath() != expectedPath {
		t.Errorf("%s does not equal %s", req.GetPath(), expectedPath)
	}
}