package k8s

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestCloudFactoryValidation(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := []struct {
		title         string
		option        *ClusterResourceFactoryOptions
		errExpected   bool
		isValid       bool
		countExpected int
		errContent    string
	}{
		{
			title: "account name, namespace, id, and cluster id empty at the same time",
			option: &ClusterResourceFactoryOptions{
				AccountName:      "",
				AccountID:        "",
				AccountNamespace: "",
				ClusterID:        "",
			},
			errExpected:   false,
			isValid:       false,
			countExpected: 0,
			errContent:    "cannot be empty at the same time",
		},
		{
			title: "account name and id set at the same time",
			option: &ClusterResourceFactoryOptions{
				AccountName: "foo",
				AccountID:   "bar",
			},
			errExpected:   true,
			isValid:       false,
			countExpected: 2,
			errContent:    "cannot be combined",
		},
		{
			title: "account name and cluster id set at the same time",
			option: &ClusterResourceFactoryOptions{
				AccountName: "foo",
				ClusterID:   "bar",
			},
			errExpected:   true,
			isValid:       false,
			countExpected: 2,
			errContent:    "cannot be combined",
		},
		{
			title: "account id and cluster id set at the same time",
			option: &ClusterResourceFactoryOptions{
				AccountID: "foo",
				ClusterID: "bar",
			},
			errExpected:   true,
			isValid:       false,
			countExpected: 2,
			errContent:    "cannot be combined",
		},
		{
			title: "succeed",
			option: &ClusterResourceFactoryOptions{
				AccountName: "foo",
			},
			errExpected:   false,
			isValid:       true,
			countExpected: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			count := tc.option.countAccountIdentifiers()
			g.Expect(count).Should(Equal(tc.countExpected), "count of identifiers doesn't match")

			valid, err := tc.option.ValidateIdentifiers()
			g.Expect(valid).Should(Equal(tc.isValid), "Boolean response doesn't match")

			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
				if tc.errContent != "" {
					g.Expect(true).Should(Equal(strings.Contains(err.Error(), tc.errContent)), "Error string does not contain content")
				}
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}
