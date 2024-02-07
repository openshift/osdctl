package network

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestPacketCaptureCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := []struct {
		title       string
		option      *packetCaptureOptions
		errExpected bool
	}{
		{
			title:       "succeed",
			option:      &packetCaptureOptions{},
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
