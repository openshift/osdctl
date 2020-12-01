package servicequotas


import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestGetSupportedRegions(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := []struct {
		title		string
		region      string
		allRegions  bool
		errExpected bool
		errContent  string
		expectRegionsMoreThanOne bool
		expectRegionsOnlyOne bool
		expectRegionsNone bool
	}{
		{
			title:       "no region filter, not all regions, no error, return all",
			region:       "",
			allRegions:   false,
			errExpected:  false,
			expectRegionsMoreThanOne: true,
			expectRegionsOnlyOne: false,
			expectRegionsNone: false,
		},
		{
			title:       "no region filter, return all regions, no error, return all",
			region:       "",
			allRegions:   true,
			errExpected:  false,
			expectRegionsMoreThanOne: true,
			expectRegionsOnlyOne: false,
			expectRegionsNone: false,
		},
		{
			title:       "us-east-1 region filter, not all regions, should return 1 region",
			region:       "us-east-1",
			allRegions:   false,
			errExpected:  false,
			expectRegionsMoreThanOne: false,
			expectRegionsOnlyOne: true,
			expectRegionsNone: false,
		},
		{
			title:       "us-east-1 region filter, return all regions, should return all regions",
			region:       "us-east-1",
			allRegions:   true,
			errExpected:  false,
			expectRegionsMoreThanOne: true,
			expectRegionsOnlyOne: false,
			expectRegionsNone: false,
		},
		{
			title:       "us-east-15 region filter, not all regions, should error",
			region:       "us-east-15",
			allRegions:   false,
			errExpected:  true,
			expectRegionsMoreThanOne: false,
			expectRegionsOnlyOne: false,
			expectRegionsNone: true,
		},
		{
			title:       "us-east-15 region filter, return all regions, should return all regions",
			region:       "us-east-15",
			allRegions:   true,
			errExpected:  false,
			expectRegionsMoreThanOne: true,
			expectRegionsOnlyOne: false,
			expectRegionsNone: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			regions, err := GetSupportedRegions(tc.region, tc.allRegions)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
				if tc.errContent != "" {
					g.Expect(true).Should(Equal(strings.Contains(err.Error(), tc.errContent)))
				}
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}

			g.Expect(tc.expectRegionsMoreThanOne == (len(regions) > 1)).To(BeTrue(), "Should return more than one region")
			g.Expect(tc.expectRegionsOnlyOne == (len(regions) == 1)).To(BeTrue(), "Should return exactly one region")
			g.Expect(tc.expectRegionsNone == (len(regions) == 0)).To(BeTrue(), "Should return no regions")
		})
	}
}
