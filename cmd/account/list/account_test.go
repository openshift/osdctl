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

func TestGetAccountCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	mockCtrl := gomock.NewController(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
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
				GlobalOptions: &globalFlags,
			},
			errExpected: true,
			errContent:  "unsupported account state foo",
		},
		{
			title: "empty state",
			option: &listAccountOptions{
				state:         "",
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "all state",
			option: &listAccountOptions{
				state:         "all",
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "Ready state",
			option: &listAccountOptions{
				state:         "Ready",
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "bad reuse",
			option: &listAccountOptions{
				reused:        "foo",
				GlobalOptions: &globalFlags,
			},
			errExpected: true,
			errContent:  "unsupported reused status filter foo",
		},
		{
			title: "bad reused status",
			option: &listAccountOptions{
				reused:        "foo",
				GlobalOptions: &globalFlags,
			},
			errExpected: true,
			errContent:  "unsupported reused status filter foo",
		},
		{
			title: "bad claimed status",
			option: &listAccountOptions{
				claimed:       "foo",
				GlobalOptions: &globalFlags,
			},
			errExpected: true,
			errContent:  "unsupported claimed status filter foo",
		},
		{
			title: "good reused true",
			option: &listAccountOptions{
				reused:        "true",
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
		{
			title: "good claim",
			option: &listAccountOptions{
				claimed:       "false",
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
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdListAccount(streams, mockk8s.NewMockClient(mockCtrl), &globalFlags)
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
