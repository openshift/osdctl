package clusterdeployment

import (
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"

	mockk8s "github.com/openshift/osdctl/cmd/clusterdeployment/mock/k8s"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestListCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	mockCtrl := gomock.NewController(t)
	kubeFlags := genericclioptions.NewConfigFlags(false)
	testCases := []struct {
		title       string
		option      *listOptions
		errExpected bool
	}{
		{
			title: "succeed",
			option: &listOptions{
				flags:   kubeFlags,
				kubeCli: mockk8s.NewMockClient(mockCtrl),
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			err := tc.option.complete(nil, nil)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}
