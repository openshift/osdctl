package capability

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestGetCapabilityKeys(t *testing.T) {

	tests := []struct {
		Name         string
		ExpectedResp string
		Input        string
	}{
		{
			Name:         "Hibernation capability key",
			ExpectedResp: "capability.organization.hibernate_cluster",
			Input:        "hibernation",
		},
		{
			Name:         "Autoscaling capability key",
			ExpectedResp: "capability.cluster.autoscale_clusters",
			Input:        "autoscaling",
		},
		{
			Name:         "Upgrade Channel capability key",
			ExpectedResp: "capability.organization.allow_set_upgrade_channel_group",
			Input:        "upgradeChannelChange",
		},
		{
			Name:         "OVN capability key",
			ExpectedResp: "capability.organization.ovn_cluster",
			Input:        "ovn",
		},
		{
			Name:         "Invalid capability key",
			ExpectedResp: "Not a valid capability",
			Input:        "foo",
		},
	}

	for _, test := range tests {
		result := getCapabilityKey(test.Input)
		if result != test.ExpectedResp {
			t.Fatalf("Test %s failed. Expected %s, got %s\n", test.Name, test.ExpectedResp, result)
		}
	}
}

func TestAddComplete(t *testing.T) {
	tests := []struct {
		Name          string
		ErrorExpected bool
		ErrorReason   string
		Args          []string
		AddOptions    addOptions
	}{
		{
			Name:          "Providing Organization ID",
			ErrorExpected: false,
			Args:          []string{"hibernation"},
			AddOptions:    addOptions{OrganizationID: "myOrganization"},
		},
		{
			Name:          "Providing Subscription ID",
			ErrorExpected: false,
			Args:          []string{"hibernation"},
			AddOptions:    addOptions{SubscriptionID: "mySubscription"},
		},
		{
			Name:          "Providing Organization ID and Subscription ID",
			ErrorExpected: true,
			ErrorReason:   "organization and subscription cannot both be set. Please only specify one",
			Args:          []string{"hibernation"},
			AddOptions:    addOptions{OrganizationID: "myorganization", SubscriptionID: "mySubscription"},
		},
		{
			Name:          "Providing neither Organization ID and Subscription ID",
			ErrorExpected: true,
			ErrorReason:   "no organization or subscription was provided, please specify organization using -g, or subscription using -b",
			Args:          []string{"hibernation"},
			AddOptions:    addOptions{},
		},
		{
			Name:          "Providing hibernation capability",
			ErrorExpected: false,
			Args:          []string{"hibernation"},
			AddOptions:    addOptions{OrganizationID: "myOrganization"},
		},
		{
			Name:          "Providing invalid capability",
			ErrorExpected: true,
			ErrorReason:   "invalid capability specified. Avaialbe capabilites are hibernation, autoscaling, ovn and upgradeChannelChange",
			Args:          []string{"invalid"},
			AddOptions:    addOptions{OrganizationID: "myOrganization"},
		},
		{
			Name:          "Providing no arguments",
			ErrorExpected: true,
			ErrorReason:   "capability was not provided, please specifiy which capability to add",
			Args:          []string{},
			AddOptions:    addOptions{OrganizationID: "myOrganization"},
		},
		{
			Name:          "Providing too many arguments",
			ErrorExpected: true,
			ErrorReason:   "too many arguments. Expected 1 got 2",
			Args:          []string{"hibernation", "foo"},
			AddOptions:    addOptions{OrganizationID: "myOrganization"},
		},
	}

	for _, test := range tests {
		result := test.AddOptions.complete(&cobra.Command{}, test.Args)
		if test.ErrorExpected {
			if result == nil {
				t.Fatalf("Test %s failed. Expected error %s, but got none", test.Name, test.ErrorReason)
			}
			if result.Error() != test.ErrorReason {
				t.Fatalf("Test %s failed. Expected error %s, but got %s", test.Name, test.ErrorReason, result.Error())
			}
		}
		if !test.ErrorExpected && result != nil {
			t.Fatalf("Test %s failed. Expected no errors, but got %s", test.Name, result.Error())
		}

	}
}

func TestRemoveComplete(t *testing.T) {
	tests := []struct {
		Name          string
		ErrorExpected bool
		ErrorReason   string
		Args          []string
		RemoveOptions removeOptions
	}{
		{
			Name:          "Providing Organization ID",
			ErrorExpected: false,
			Args:          []string{"hibernation"},
			RemoveOptions: removeOptions{OrganizationID: "myOrganization"},
		},
		{
			Name:          "Providing Subscription ID",
			ErrorExpected: false,
			Args:          []string{"hibernation"},
			RemoveOptions: removeOptions{SubscriptionID: "mySubscription"},
		},
		{
			Name:          "Providing Organization ID and Subscription ID",
			ErrorExpected: true,
			ErrorReason:   "organization and subscription cannot both be set. Please only specify one",
			Args:          []string{"hibernation"},
			RemoveOptions: removeOptions{OrganizationID: "myorganization", SubscriptionID: "mySubscription"},
		},
		{
			Name:          "Providing neither Organization ID and Subscription ID",
			ErrorExpected: true,
			ErrorReason:   "no organization or subscription was provided, please specify organization using -g, or subscription using -b",
			Args:          []string{"hibernation"},
			RemoveOptions: removeOptions{},
		},
		{
			Name:          "Providing hibernation capability",
			ErrorExpected: false,
			Args:          []string{"hibernation"},
			RemoveOptions: removeOptions{OrganizationID: "myOrganization"},
		},
		{
			Name:          "Providing invalid capability",
			ErrorExpected: true,
			ErrorReason:   "invalid capability specified. Avaialbe capabilites are hibernation, autoscaling, ovn and upgradeChannelChange",
			Args:          []string{"invalid"},
			RemoveOptions: removeOptions{OrganizationID: "myOrganization"},
		},
		{
			Name:          "Providing no arguments",
			ErrorExpected: true,
			ErrorReason:   "capability was not provided, please specifiy which capability to add",
			Args:          []string{},
			RemoveOptions: removeOptions{OrganizationID: "myOrganization"},
		},
		{
			Name:          "Providing too many arguments",
			ErrorExpected: true,
			ErrorReason:   "too many arguments. Expected 1 got 2",
			Args:          []string{"hibernation", "foo"},
			RemoveOptions: removeOptions{OrganizationID: "myOrganization"},
		},
	}

	for _, test := range tests {
		result := test.RemoveOptions.complete(&cobra.Command{}, test.Args)
		if test.ErrorExpected {
			if result == nil {
				t.Fatalf("Test %s failed. Expected error %s, but got none", test.Name, test.ErrorReason)
			}
			if result.Error() != test.ErrorReason {
				t.Fatalf("Test %s failed. Expected error %s, but got %s", test.Name, test.ErrorReason, result.Error())
			}
		}
		if !test.ErrorExpected && result != nil {
			t.Fatalf("Test %s failed. Expected no errors, but got %s", test.Name, result.Error())
		}
	}
}
