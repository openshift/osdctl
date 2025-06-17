package iampermissions

import (
	"bytes"
	"errors"
	"testing"

	"github.com/openshift/osdctl/pkg/policies"
	"github.com/stretchr/testify/assert"
)

func TestRun_Success(t *testing.T) {
	var outputBuffer bytes.Buffer

	opts := &getOptions{
		ReleaseVersion: "4.15.0",
		Cloud:          policies.AWS,
		downloadFunc: func(version string, cloud policies.CloudSpec) (string, error) {
			assert.Equal(t, "4.15.0", version)
			assert.Equal(t, policies.AWS, cloud)
			return "/mock/dir", nil
		},
		outputWriter: &outputBuffer,
	}

	err := opts.run()
	assert.NoError(t, err)
	assert.Contains(t, outputBuffer.String(), "OCP CredentialsRequests for aws have been saved in /mock/dir")
}

func TestRun_Failure(t *testing.T) {
	opts := &getOptions{
		ReleaseVersion: "invalid",
		Cloud:          policies.AWS,
		downloadFunc: func(version string, cloud policies.CloudSpec) (string, error) {
			return "", errors.New("download failed")
		},
		outputWriter: &bytes.Buffer{},
	}

	err := opts.run()
	assert.Error(t, err)
	assert.Equal(t, "download failed", err.Error())
}
