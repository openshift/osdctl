package federatedrole

import (
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"

	mockk8s "github.com/openshift/osdctl/cmd/clusterdeployment/mock/k8s"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestListCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	mockCtrl := gomock.NewController(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	kubeFlags := genericclioptions.NewConfigFlags(false)
	testCases := []struct {
		title       string
		option      *applyOptions
		errExpected bool
		errContent  string
	}{
		{
			title: "url and file specified at the same time",
			option: &applyOptions{
				url:  "http://example.com",
				file: "foo",
			},
			errExpected: true,
			errContent:  "Flags file and url cannot be set at the same time",
		},
		{
			title: "url and file empty at the same time",
			option: &applyOptions{
				url:  "",
				file: "",
			},
			errExpected: true,
			errContent:  "Flags file and url cannot be empty at the same time",
		},
		{
			title: "success",
			option: &applyOptions{
				url:   "foo",
				flags: kubeFlags,
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdApply(streams, kubeFlags, mockk8s.NewMockClient(mockCtrl))
			err := tc.option.complete(cmd, nil)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}
