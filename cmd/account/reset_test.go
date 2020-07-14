package account

import (
	"os"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestResetCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	kubeFlags := genericclioptions.NewConfigFlags(false)
	testCases := []struct {
		title       string
		option      *resetOptions
		args        []string
		errExpected bool
		errContent  string
	}{
		{
			title:       "no args provided",
			args:        []string{},
			errExpected: true,
			errContent:  "The name of Account CR is required for reset command",
		},
		{
			title:       "two args provided",
			args:        []string{"foo", "bar"},
			errExpected: true,
			errContent:  "The name of Account CR is required for reset command",
		},
		{
			title: "succeed",
			option: &resetOptions{
				flags: kubeFlags,
			},
			args:        []string{"foo"},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdReset(streams, kubeFlags)
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
