package list

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
		option      *listAccountOptions
		errExpected bool
		errContent  string
	}{
		{
			title: "incorrect state",
			option: &listAccountOptions{
				state:         "foo",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: true,
			errContent:  "unsupported account state foo",
		},
		{
			title: "empty state",
			option: &listAccountOptions{
				state:         "",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "all state",
			option: &listAccountOptions{
				state:         "all",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "Ready state",
			option: &listAccountOptions{
				state:         "Ready",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "bad reuse",
			option: &listAccountOptions{
				reused:        "foo",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: true,
			errContent:  "unsupported reused status filter foo",
		},
		{
			title: "bad reused status",
			option: &listAccountOptions{
				reused:        "foo",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: true,
			errContent:  "unsupported reused status filter foo",
		},
		{
			title: "bad claimed status",
			option: &listAccountOptions{
				claimed:       "foo",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: true,
			errContent:  "unsupported claimed status filter foo",
		},
		{
			title: "good reused true",
			option: &listAccountOptions{
				reused:        "true",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "good claim",
			option: &listAccountOptions{
				claimed:       "false",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "success",
			option: &listAccountOptions{
				state:         "Ready",
				reused:        "true",
				claimed:       "false",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdListAccount(streams, kubeFlags, &globalFlags)
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
