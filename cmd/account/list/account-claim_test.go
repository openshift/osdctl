package list

import (
	"os"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	mockk8s "github.com/openshift/osdctl/cmd/hive/clusterdeployment/mock/k8s"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestGetAccountClaimCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	mockCtrl := gomock.NewController(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	globalFlags := globalflags.GlobalOptions{Output: ""}
	testCases := []struct {
		title       string
		option      *listAccountClaimOptions
		errExpected bool
		errContent  string
	}{
		{
			title: "incorrect state",
			option: &listAccountClaimOptions{
				state:         "foo",
				GlobalOptions: &globalFlags,
			},
			errExpected: true,
			errContent:  "unsupported account claim state foo",
		},
		{
			title: "empty state",
			option: &listAccountClaimOptions{
				state:         "",
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "error state",
			option: &listAccountClaimOptions{
				state:         "Error",
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "pending state",
			option: &listAccountClaimOptions{
				state:         "Pending",
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "ready state",
			option: &listAccountClaimOptions{
				state:         "Ready",
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdListAccountClaim(streams, mockk8s.NewMockClient(mockCtrl), &globalFlags)
			err := tc.option.complete(cmd, nil)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
				if tc.errContent != "" {
					g.Expect(true).Should(Equal(strings.Contains(err.Error(), tc.errContent)))
				}
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}
