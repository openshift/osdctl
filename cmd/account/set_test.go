package account

import (
	"os"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"

	mockk8s "github.com/openshift/osdctl/cmd/hive/clusterdeployment/mock/k8s"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestSetCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	mockCtrl := gomock.NewController(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	testCases := []struct {
		title       string
		option      *setOptions
		args        []string
		errExpected bool
		errContent  string
	}{
		{
			title:       "no args provided",
			args:        []string{},
			errExpected: true,
			errContent:  "The name of Account CR is required for set command",
		},
		{
			title:       "two args provided",
			args:        []string{"foo", "bar"},
			errExpected: true,
			errContent:  "The name of Account CR is required for set command",
		},
		{
			title: "invalid state specified",
			option: &setOptions{
				state: "bar",
			},
			args:        []string{"foo"},
			errExpected: true,
			errContent:  "unsupported account state bar",
		},
		{
			title: "succeed",
			option: &setOptions{
				state: "Creating",
			},
			args:        []string{"foo"},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdSet(streams, mockk8s.NewMockClient(mockCtrl))
			err := tc.option.complete(cmd, tc.args)
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
