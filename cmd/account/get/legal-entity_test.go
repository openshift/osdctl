package get

import (
	"os"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"

	mockk8s "github.com/openshift/osdctl/cmd/hive/clusterdeployment/mock/k8s"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestGetLegalEntityCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	mockCtrl := gomock.NewController(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	kubeFlags := genericclioptions.NewConfigFlags(false)
	globalFlags := globalflags.GlobalOptions{Output: ""}
	testCases := []struct {
		title       string
		option      *getSecretsOptions
		errExpected bool
		errContent  string
	}{
		{
			title: "empty account id",
			option: &getSecretsOptions{
				accountID:     "",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: true,
			errContent:  accountIDRequired,
		},
		{
			title: "succeed",
			option: &getSecretsOptions{
				accountID:     "foo",
				flags:         kubeFlags,
				GlobalOptions: &globalFlags,
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdGetLegalEntity(streams, kubeFlags, mockk8s.NewMockClient(mockCtrl), &globalFlags)
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
