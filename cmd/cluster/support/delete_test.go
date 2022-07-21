package support

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestCreateDeleteRequest(t *testing.T) {
	g := NewGomegaWithT(t)

	testCases := []struct {
		title          string
		clusterID      string
		lmtSprReasonID string
		client         *Client
		errExpected    bool
	}{
		{
			title:          "Delete Request creation",
			clusterID:      "5a5a5a5a-5a5a-5a5a-5a5a-5a5a5a5a5a5a",
			lmtSprReasonID: "FAFAFAFA-FAFAFAFA-FAFAFAFA-FAFAFAFA",
			client: &Client{
				name: "fakeSDKClient",
			},
			errExpected: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			_, err := createDeleteRequest(tc.client, tc.clusterID, tc.lmtSprReasonID)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}
