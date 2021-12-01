package get

import (
	"os"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift/osdctl/internal/utils/globalflags"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestGetAccountCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	kubeFlags := genericclioptions.NewConfigFlags(false)
	globalFlags := globalflags.GlobalOptions{Output: ""}
	testCases := []struct {
		title       string
		option      *getAccountOptions
		errExpected bool
		errContent  string
	}{
		{
			title: "account id and account claim cr name empty at the same time",
			option: &getAccountOptions{
				accountID:        "",
				accountClaimName: "",
				flags:            kubeFlags,
				GlobalOptions:    &globalFlags,
			},
			errExpected: true,
			errContent:  "AWS account ID and AccountClaim CR Name cannot be empty at the same time",
		},
		{
			title: "account id and account claim cr name set at the same time",
			option: &getAccountOptions{
				accountID:        "foo",
				accountClaimName: "bar",
				flags:            kubeFlags,
				GlobalOptions:    &globalFlags,
			},
			errExpected: true,
			errContent:  "AWS account ID and AccountClaim CR Name cannot be set at the same time",
		},
		{
			title: "succeed",
			option: &getAccountOptions{
				accountID:     "foo",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "succeed with account claim",
			option: &getAccountOptions{
				accountClaimName: "foo",
				flags:            kubeFlags,
				GlobalOptions:    &globalFlags,
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdGetAccount(streams, kubeFlags, &globalFlags)
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
