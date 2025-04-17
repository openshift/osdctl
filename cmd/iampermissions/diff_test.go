package iampermissions

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/openshift/osdctl/pkg/policies"
	"github.com/stretchr/testify/assert"
)

func TestRunSuccess(t *testing.T) {
	mockDownload := func(version string, cloud policies.CloudSpec) (string, error) {
		return "/mock/path/" + version, nil
	}

	mockExec := func(command string, args ...string) *exec.Cmd {
		return exec.Command("echo", "mock diff output")
	}

	var outputBuffer bytes.Buffer

	o := &diffOptions{
		BaseVersion:   "v1",
		TargetVersion: "v2",
		Cloud:         policies.AWS,
		downloadFunc:  mockDownload,
		execFunc:      mockExec,
		outputWriter:  &outputBuffer,
	}
	err := o.run()
	assert.NoError(t, err)
	assert.Contains(t, outputBuffer.String(), "mock diff output")
}
