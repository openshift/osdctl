package account

import (
	"os"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestCheckSecretsCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	kubeFlags := genericclioptions.NewConfigFlags(false)
	testCases := []struct {
		title       string
		option      *checkSecretsOptions
		args        []string
		errExpected bool
		errContent  string
	}{
		{
			title:       "two args provided",
			args:        []string{"foo", "bar"},
			errExpected: true,
			errContent:  checkSecretsUsage,
		},
		{
			title: "succeed with one arg",
			option: &checkSecretsOptions{
				accountName: "foo",
				flags:       kubeFlags,
			},
			args:        []string{"foo"},
			errExpected: false,
		},
		{
			title: "succeed with one arg",
			option: &checkSecretsOptions{
				flags: kubeFlags,
			},
			args:        []string{},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdCheckSecrets(streams, kubeFlags)
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
