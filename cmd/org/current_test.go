package org

import (
	"testing"

	"github.com/openshift/osdctl/pkg/utils"
)

var userData = `
{
    "created_at": "2024-11-27T07:32:59.368563Z",
    "email": "kilgore.trout@redhat.com",
    "first_name": "Kilgore",
    "href": "/api/accounts_mgmt/v1/accounts/xyz",
    "id": "foobar",
    "kind": "Account",
    "last_name": "Trout",
    "organization": {
        "created_at": "2019-02-15T20:26:12.542449Z",
        "ebs_account_id": "1111111",
        "external_id": "2222222",
        "href": "/api/accounts_mgmt/v1/organizations/xyz",
        "id": "abc.xyz",
        "kind": "Organization",
        "name": "Kurt Vonnegut Appreciation Society",
        "updated_at": "2025-03-10T06:16:08.047253Z"
    },
    "rhit_web_user_id": "57712380",
    "updated_at": "2025-02-21T21:05:50.544761Z",
    "username": "kilgore.trout"
}
`

func TestGetCurrentOrg(t *testing.T) {
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
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		t.Fatal(err)
	}
	req, err := getOrgRequest(ocmClient)
	if err != nil {
		t.Fatal(err)
	}
	if req.GetPath() != currentAccountApiPath {
		t.Errorf("%s does not equal %s", req.GetPath(), currentAccountApiPath)
	}
}
