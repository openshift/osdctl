package clusterdeployment

import (
	"testing"

	. "github.com/onsi/gomega"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestResetCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	kubeFlags := genericclioptions.NewConfigFlags(false)
	testCases := []struct {
		title       string
		option      *listOptions
		errExpected bool
	}{
		{
			title: "succeed",
			option: &listOptions{
				flags: kubeFlags,
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
