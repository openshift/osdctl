package support

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/openshift/osdctl/internal/utils/globalflags"
)

func TestReadTemplate(t *testing.T) {
	g := NewGomegaWithT(t)

	testCases := []struct {
		title       string
		option      *postOptions
		errExpected bool
	}{
		{
			title: "No template",
			option: &postOptions{
				GlobalOptions: &globalflags.GlobalOptions{},
			},
			errExpected: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			err := readTemplate()
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}

func TestAccessFile(t *testing.T) {
	g := NewGomegaWithT(t)

	testCases := []struct {
		title       string
		path        string
		errExpected bool
	}{
		{
			title:       "File not existant",
			path:        "/tmp/fakeFile.json",
			errExpected: true,
		},
		{
			title:       "File existant",
			path:        "./post.go",
			errExpected: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			_, err := accessFile(tc.path)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}

func TestCreatePostRequest(t *testing.T) {
	g := NewGomegaWithT(t)

	testCases := []struct {
		title       string
		clusterID   string
		client      *Client
		errExpected bool
	}{
		{
			title:     "Post Request creation",
			clusterID: "5a5a5a5a-5a5a-5a5a-5a5a-5a5a5a5a5a5a",
			client: &Client{
				name: "fakeSDKClient",
			},
			errExpected: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			_, err := createPostRequest(tc.client, tc.clusterID)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}

func TestValidateBadResponse(t *testing.T) {
	g := NewGomegaWithT(t)

	gMsg := "{\"field1\":\"data1\",\"field2\":\"data2\"}"
	bMsg := "{\"field1\":\"data1\",\"field2\"\"data2\"}"

	testCases := []struct {
		title       string
		body        []byte
		errExpected bool
	}{
		{
			title:       "Validate good Json Response",
			body:        []byte(gMsg),
			errExpected: false,
		},
		{
			title:       "Validate bad Json Response",
			body:        []byte(bMsg),
			errExpected: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			_, err := validateBadResponse(tc.body)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}
