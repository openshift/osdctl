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

type Flags struct {
	configFlags *genericclioptions.ConfigFlags
	output      string
}

func TestGetAccountCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	mockCtrl := gomock.NewController(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	var flags Flags
	flags.configFlags = genericclioptions.NewConfigFlags(false)
	flags.output = "text"

	testCases := []struct {
		title       string
		option      *listAccountOptions
		errExpected bool
		errContent  string
	}{
		{
			title: "incorrect state",
			option: &listAccountOptions{
				state: "foo",
			},
			errExpected: true,
			errContent:  "unsupported account state foo",
		},
		{
			title: "empty state",
			option: &listAccountOptions{
				state: "",
				flags: flags.configFlags,
			},
			errExpected: false,
		},
		{
			title: "all state",
			option: &listAccountOptions{
				state: "all",
				flags: flags.configFlags,
			},
			errExpected: false,
		},
		{
			title: "Ready state",
			option: &listAccountOptions{
				state: "Ready",
				flags: flags.configFlags,
			},
			errExpected: false,
		},
		{
			title: "bad reuse",
			option: &listAccountOptions{
				reused: "foo",
			},
			errExpected: true,
			errContent:  "unsupported reused status filter foo",
		},
		{
			title: "bad reused status",
			option: &listAccountOptions{
				reused: "foo",
			},
			errExpected: true,
			errContent:  "unsupported reused status filter foo",
		},
		{
			title: "bad claimed status",
			option: &listAccountOptions{
				claimed: "foo",
			},
			errExpected: true,
			errContent:  "unsupported claimed status filter foo",
		},
		{
			title: "good reused true",
			option: &listAccountOptions{
				reused: "true",
				flags:  flags.configFlags,
			},
			errExpected: false,
		},
		{
			title: "good claim",
			option: &listAccountOptions{
				claimed: "false",
				flags:   flags.configFlags,
			},
			errExpected: false,
		},
		{
			title: "success",
			option: &listAccountOptions{
				state:   "Ready",
				reused:  "true",
				claimed: "false",
				flags:   flags.configFlags,
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdListAccount(streams, flags.configFlags, mockk8s.NewMockClient(mockCtrl))
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
