package iampermissions

import (
	"fmt"
	"os"
	"testing"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	"github.com/openshift/osdctl/pkg/policies"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSaveOptions_Run_Success_AWS(t *testing.T) {
	var printed []string

	cr := &cco.CredentialsRequest{ObjectMeta: metav1.ObjectMeta{Name: "test-cr"}}

	opts := &saveOptions{
		OutFolder:      "out",
		ReleaseVersion: "v1",
		Cloud:          policies.AWS,
		Force:          true,

		DownloadCRs: func(version string, cloud policies.CloudSpec) (string, error) {
			assert.Equal(t, "v1", version)
			assert.Equal(t, policies.AWS, cloud)
			return "/fake-dir", nil
		},

		ParseCRsInDir: func(dir string) ([]*cco.CredentialsRequest, error) {
			assert.Equal(t, "/fake-dir", dir)
			return []*cco.CredentialsRequest{cr}, nil
		},

		AWSConverter: func(req *cco.CredentialsRequest) (*policies.PolicyDocument, error) {
			assert.Equal(t, cr, req)
			return &policies.PolicyDocument{Version: "1.0"}, nil
		},

		GCPConverter: func(req *cco.CredentialsRequest) (*policies.ServiceAccount, error) {
			t.Fatalf("GCPConverter called for AWS test")
			return nil, nil
		},

		MkdirAll: func(path string, perm os.FileMode) error {
			assert.Equal(t, "out", path)
			return nil
		},

		Stat: func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},

		WriteFile: func(name string, data []byte, perm os.FileMode) error {
			assert.Equal(t, "out/test-cr.json", name)
			assert.Equal(t, os.FileMode(0600), perm)
			assert.Contains(t, string(data), "Version")
			return nil
		},

		Print: func(format string, a ...interface{}) (int, error) {
			printed = append(printed, fmt.Sprintf(format, a...))
			return 0, nil
		},
	}

	err := opts.run()
	assert.NoError(t, err)

	assert.Contains(t, printed, "Writing out/test-cr.json\n")
}

func TestSaveOptions_Run_Success_GCP(t *testing.T) {
	var printed []string
	cr := &cco.CredentialsRequest{ObjectMeta: metav1.ObjectMeta{Name: "test-cr-gcp"}}

	opts := &saveOptions{
		OutFolder:      "out",
		ReleaseVersion: "v2",
		Cloud:          policies.GCP,
		Force:          true,

		DownloadCRs: func(version string, cloud policies.CloudSpec) (string, error) {
			assert.Equal(t, "v2", version)
			assert.Equal(t, policies.GCP, cloud)
			return "/fake-dir", nil
		},

		ParseCRsInDir: func(dir string) ([]*cco.CredentialsRequest, error) {
			assert.Equal(t, "/fake-dir", dir)
			return []*cco.CredentialsRequest{cr}, nil
		},

		AWSConverter: func(req *cco.CredentialsRequest) (*policies.PolicyDocument, error) {
			t.Fatalf("AWSConverter should not be called in GCP test")
			return nil, nil
		},

		GCPConverter: func(req *cco.CredentialsRequest) (*policies.ServiceAccount, error) {
			assert.Equal(t, cr, req)
			return &policies.ServiceAccount{Id: "sa-id"}, nil
		},

		MkdirAll: func(path string, perm os.FileMode) error {
			assert.Equal(t, "out", path)
			return nil
		},

		Stat: func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},

		WriteFile: func(name string, data []byte, perm os.FileMode) error {
			assert.Equal(t, "out/sa-id.yaml", name)
			assert.Contains(t, string(data), "sa-id")
			return nil
		},

		Print: func(format string, a ...interface{}) (int, error) {
			printed = append(printed, fmt.Sprintf(format, a...))
			return 0, nil
		},
	}

	err := opts.run()
	assert.NoError(t, err)
	assert.Contains(t, printed, "Writing out/sa-id.yaml\n")
}
