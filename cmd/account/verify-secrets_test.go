package account

import (
	"os"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"

	mockk8s "github.com/openshift/osdctl/cmd/clusterdeployment/mock/k8s"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestCheckSecretsCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	mockCtrl := gomock.NewController(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	kubeFlags := genericclioptions.NewConfigFlags(false)
	testCases := []struct {
		title       string
		option      *verifySecretsOptions
		args        []string
		errExpected bool
		errContent  string
	}{
		{
			title:       "two args provided",
			args:        []string{"foo", "bar"},
			errExpected: true,
			errContent:  verifySecretsUsage,
		},
		{
			title: "succeed with one arg",
			option: &verifySecretsOptions{
				accountName: "foo",
				flags:       kubeFlags,
			},
			args:        []string{"foo"},
			errExpected: false,
		},
		{
			title: "succeed with one arg",
			option: &verifySecretsOptions{
				flags: kubeFlags,
			},
			args:        []string{},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdVerifySecrets(streams, kubeFlags, mockk8s.NewMockClient(mockCtrl))
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
