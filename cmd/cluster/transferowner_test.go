package cluster

import (
	"testing"

	. "github.com/onsi/gomega"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
)

func TestValidateOldOwner(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := []struct {
		title             string
		subscription      *amv1.SubscriptionBuilder
		oldOwner          *amv1.AccountBuilder
		oldOrganizationId string
		creator           *amv1.AccountBuilder
		okExpected        bool
	}{
		{
			title:             "valid old owner",
			oldOwner:          amv1.NewAccount().ID("test"),
			oldOrganizationId: "test",
			creator:           amv1.NewAccount().ID("test"),
			subscription:      amv1.NewSubscription().OrganizationID("test"),
			okExpected:        true,
		},
		{
			title:             "old-organization-id differs on subscription",
			oldOwner:          amv1.NewAccount().ID("test"),
			oldOrganizationId: "123",
			creator:           amv1.NewAccount().ID("test"),
			subscription:      amv1.NewSubscription().OrganizationID("test"),
			okExpected:        false,
		},
		{
			title:             "old owner differs on subscription",
			oldOwner:          amv1.NewAccount().ID("test"),
			oldOrganizationId: "123",
			creator:           amv1.NewAccount().ID("123"),
			subscription:      amv1.NewSubscription().OrganizationID("test"),
			okExpected:        false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			acc, _ := tc.oldOwner.Build()
			sub, _ := tc.subscription.Creator(tc.creator).Build()
			ok := validateOldOwner(tc.oldOrganizationId, sub, acc)
			if tc.okExpected {
				g.Expect(ok).Should(BeTrue())
			} else {
				g.Expect(ok).ShouldNot(BeTrue())
			}
		})
	}
}
