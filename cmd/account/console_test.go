package account

import (
	"os"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestConsoleCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	kubeFlags := genericclioptions.NewConfigFlags(false)
	testCases := []struct {
		title       string
		option      *consoleOptions
		errExpected bool
		errContent  string
	}{
		{
			title: "account name, id, and cluster id empty at the same time",
			option: &consoleOptions{
				accountName: "",
				accountID:   "",
				clusterID:   "",
			},
			errExpected: true,
			errContent:  "cannot be empty at the same time",
		},
		{
			title: "account name and id set at the same time",
			option: &consoleOptions{
				accountName: "foo",
				accountID:   "bar",
			},
			errExpected: true,
			errContent:  "cannot be set at the same time",
		},
		{
			title: "account name and cluster id set at the same time",
			option: &consoleOptions{
				accountName: "foo",
				clusterID:   "bar",
			},
			errExpected: true,
			errContent:  "cannot be set at the same time",
		},
		{
			title: "account id and cluster id set at the same time",
			option: &consoleOptions{
				accountID: "foo",
				clusterID: "bar",
			},
			errExpected: true,
			errContent:  "cannot be set at the same time",
		},
		{
			title: "succeed",
			option: &consoleOptions{
				accountName: "foo",
				flags:       kubeFlags,
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdConsole(streams, kubeFlags)
			err := tc.option.complete(cmd)
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
