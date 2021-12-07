package list

import (
	"os"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"

	mockk8s "github.com/openshift/osdctl/cmd/clusterdeployment/mock/k8s"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestGetAccountClaimCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	mockCtrl := gomock.NewController(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	kubeFlags := genericclioptions.NewConfigFlags(false)
	testCases := []struct {
		title       string
		option      *listAccountClaimOptions
		errExpected bool
		errContent  string
	}{
		{
			title: "incorrect state",
			option: &listAccountClaimOptions{
				state: "foo",
			},
			errExpected: true,
			errContent:  "unsupported account claim state foo",
		},
		{
			title: "empty state",
			option: &listAccountClaimOptions{
				state: "",
				flags: kubeFlags,
			},
			errExpected: false,
		},
		{
			title: "error state",
			option: &listAccountClaimOptions{
				state: "Error",
				flags: kubeFlags,
			},
			errExpected: false,
		},
		{
			title: "pending state",
			option: &listAccountClaimOptions{
				state: "Pending",
				flags: kubeFlags,
			},
			errExpected: false,
		},
		{
			title: "ready state",
			option: &listAccountClaimOptions{
				state: "Ready",
				flags: kubeFlags,
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdListAccountClaim(streams, kubeFlags, mockk8s.NewMockClient(mockCtrl))
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
