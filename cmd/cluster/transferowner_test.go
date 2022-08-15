package cluster

import (
	"os"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"

	"github.com/openshift/osdctl/internal/utils/globalflags"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestTransferOwnerCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	flags := genericclioptions.ConfigFlags{}
	globalFlags := globalflags.GlobalOptions{Output: ""}
	testCases := []struct {
		title       string
		option      *transferOwnerOptions
		errExpected bool
		errContent  string
	}{
		{
			title: "invalid old-organization-id",
			option: &transferOwnerOptions{
				clusterID:         "test",
				newOwnerName:      "test",
				newOrganizationId: "2HdaaF0f2YHWwhzt3gHtT5Mja7M",
				oldOwnerName:      "test",
				oldOrganizationId: "najsnd1asd",
				dryrun:            true,
				GlobalOptions:     &globalFlags,
			},
			errExpected: true,
			errContent:  "error validating old organization-id",
		},
		{
			title: "invalid new-organization-id",
			option: &transferOwnerOptions{
				clusterID:         "test",
				newOwnerName:      "test",
				newOrganizationId: "test",
				oldOwnerName:      "test",
				oldOrganizationId: "2HdaaF0f2YHWwhzt3gHtT5Mja7M",
				dryrun:            true,
				GlobalOptions:     &globalFlags,
			},
			errExpected: true,
			errContent:  "error validating new organization-id",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdTransferOwner(streams, &flags, &globalFlags)
			err := tc.option.complete(cmd, nil)
			if tc.errExpected {
				if tc.errContent != "" {
					g.Expect(true).Should(Equal(strings.Contains(err.Error(), tc.errContent)))
				}
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}

func TestValidateOldOwner(t *testing.T) {
	g := NewGomegaWithT(t)
	globalFlags := globalflags.GlobalOptions{Output: ""}
	testCases := []struct {
		title        string
		option       *transferOwnerOptions
		subscription *amv1.SubscriptionBuilder
		oldOwner     *amv1.AccountBuilder
		creator      *amv1.AccountBuilder
		okExpected   bool
	}{
		{
			title: "valid old owner",
			option: &transferOwnerOptions{
				clusterID:         "test",
				newOwnerName:      "test",
				newOrganizationId: "2HdaaF0f2YHWwhzt3gHtT5Mja7M",
				oldOwnerName:      "test",
				oldOrganizationId: "test",
				dryrun:            true,
				GlobalOptions:     &globalFlags,
			},
			oldOwner:     amv1.NewAccount().ID("test"),
			creator:      amv1.NewAccount().ID("test"),
			subscription: amv1.NewSubscription().OrganizationID("test"),
			okExpected:   true,
		},
		{
			title: "old-organization-id differs on subscription",
			option: &transferOwnerOptions{
				clusterID:         "test",
				newOwnerName:      "test",
				newOrganizationId: "2HdaaF0f2YHWwhzt3gHtT5Mja7M",
				oldOwnerName:      "test",
				oldOrganizationId: "123",
				dryrun:            true,
				GlobalOptions:     &globalFlags,
			},
			oldOwner:     amv1.NewAccount().ID("test"),
			creator:      amv1.NewAccount().ID("test"),
			subscription: amv1.NewSubscription().OrganizationID("test"),
			okExpected:   false,
		},
		{
			title: "old owner differs on subscription",
			option: &transferOwnerOptions{
				clusterID:         "test",
				newOwnerName:      "test",
				newOrganizationId: "2HdaaF0f2YHWwhzt3gHtT5Mja7M",
				oldOwnerName:      "test",
				oldOrganizationId: "123",
				dryrun:            true,
				GlobalOptions:     &globalFlags,
			},
			oldOwner:     amv1.NewAccount().ID("test"),
			creator:      amv1.NewAccount().ID("123"),
			subscription: amv1.NewSubscription().OrganizationID("test"),
			okExpected:   false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			acc, _ := tc.oldOwner.Build()
			sub, _ := tc.subscription.Creator(tc.creator).Build()
			ok := validateOldOwner(tc.option, sub, acc)
			if tc.okExpected {
				g.Expect(ok).Should(BeTrue())
			} else {
				g.Expect(ok).ShouldNot(BeTrue())
			}
		})
	}
}
