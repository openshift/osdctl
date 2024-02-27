package clusterdeployment

import (
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"

	mockk8s "github.com/openshift/osdctl/cmd/hive/clusterdeployment/mock/k8s"
)

func TestListCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	mockCtrl := gomock.NewController(t)
	testCases := []struct {
		title       string
		option      *listOptions
		errExpected bool
	}{
		{
			title: "succeed",
			option: &listOptions{
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
