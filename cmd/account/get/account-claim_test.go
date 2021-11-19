package get

import (
	"os"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"

	mockk8s "github.com/openshift/osdctl/cmd/clusterdeployment/mock/k8s"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestGetAccountClaimCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	mockCtrl := gomock.NewController(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	kubeFlags := genericclioptions.NewConfigFlags(false)
	testCases := []struct {
		title       string
		option      *getAccountClaimOptions
		errExpected bool
		errContent  string
	}{
		{
			title: "account id and account cr name empty at the same time",
			option: &getAccountClaimOptions{
				accountID:   "",
				accountName: "",
			},
			errExpected: true,
			errContent:  "AWS account ID and Account CR Name cannot be empty at the same time",
		},
		{
			title: "account id and account cr name set at the same time",
			option: &getAccountClaimOptions{
				accountID:   "foo",
				accountName: "bar",
			},
			errExpected: true,
			errContent:  "AWS account ID and Account CR Name cannot be set at the same time",
		},
		{
			title: "succeed",
			option: &getAccountClaimOptions{
				accountID: "foo",
				flags:     kubeFlags,
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdGetAccountClaim(streams, kubeFlags, mockk8s.NewMockClient(mockCtrl))
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
