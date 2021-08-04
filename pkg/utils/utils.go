package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func GetOCMAccessToken() (*string, error) {
	// Get ocm access token
	ocmCmd := exec.Command("ocm", "token")
	ocmCmd.Stderr = os.Stderr
	ocmOutput, err := ocmCmd.Output()
	if err != nil { // Throw error if ocm not in PATH, or ocm command exit non-zero.
		return nil, fmt.Errorf("failed running ocm token: %v", err)
	}
	accessToken := strings.TrimSuffix(string(ocmOutput), "\n")

	return &accessToken, nil
}
