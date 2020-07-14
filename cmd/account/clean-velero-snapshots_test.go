package account

import (
	"os"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestCleanVeleroSnapshotsCmdComplete(t *testing.T) {
	g := NewGomegaWithT(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	testCases := []struct {
		title       string
		option      *cleanVeleroSnapshotsOptions
		errExpected bool
		errContent  string
	}{
		{
			title: "succeed with empty credentials provided",
			option: &cleanVeleroSnapshotsOptions{
				IOStreams:       streams,
				accessKeyID:     "",
				secretAccessKey: "",
			},
			errExpected: false,
		},
		{
			title: "provide accessKeyID but not secretAccessKey",
			option: &cleanVeleroSnapshotsOptions{
				IOStreams:       streams,
				accessKeyID:     "foo",
				secretAccessKey: "",
			},
			errExpected: true,
			errContent:  cleanVeleroSnapshotsUsage,
		},
		{
			title: "provide secretAccessKey but not accessKeyID",
			option: &cleanVeleroSnapshotsOptions{
				IOStreams:       streams,
				accessKeyID:     "",
				secretAccessKey: "foo",
			},
			errExpected: true,
			errContent:  cleanVeleroSnapshotsUsage,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			cmd := newCmdCleanVeleroSnapshots(streams)
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
