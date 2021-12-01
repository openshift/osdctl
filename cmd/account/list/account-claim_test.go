package list

import (
	"os"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift/osdctl/internal/utils/globalflags"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestGetAccountClaimCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	kubeFlags := genericclioptions.NewConfigFlags(false)
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
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: true,
			errContent:  "unsupported account claim state foo",
		},
		{
			title: "empty state",
			option: &listAccountClaimOptions{
				state:         "",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "error state",
			option: &listAccountClaimOptions{
				state:         "Error",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "pending state",
			option: &listAccountClaimOptions{
				state:         "Pending",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "ready state",
			option: &listAccountClaimOptions{
				state:         "Ready",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdListAccountClaim(streams, kubeFlags, &globalFlags)
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
