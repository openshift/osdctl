package dynatrace

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

func CheckoutAndCompareGitHash(gitURL, gitHash, currentGitHash, serviceFullPath string) (string, string, error) {
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	err = os.Chdir(tempDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to change directory to temporary directory: %v", err)
	}

	cmd := exec.Command("git", "clone", gitURL, "source-dir")
	err = cmd.Run()
	if err != nil {
		return "", "", fmt.Errorf("failed to clone git repository: %v", err)
	}

	err = os.Chdir("source-dir")
	if err != nil {
		return "", "", fmt.Errorf("failed to change directory to source-dir: %v", err)
	}

	if gitHash == "" {
		fmt.Printf("No git hash provided. Using HEAD.\n")
		cmd := exec.Command("git", "rev-list", "-n", "1", "HEAD", "--", serviceFullPath)
		output, err := cmd.Output()
		if err != nil {
			return "", "", fmt.Errorf("failed to get git hash: %v", err)
		}
		gitHash = strings.TrimSpace(string(output))
		fmt.Printf("The head githash is %s\n", gitHash)
	}

	if currentGitHash == gitHash {
		return "", "", fmt.Errorf("git hash %s is already at HEAD", gitHash)
	} else {
		cmd := exec.Command("git", "log", "--no-merges", fmt.Sprintf("%s..%s", currentGitHash, gitHash))
		commitLog, err := cmd.Output()
		if err != nil {
			return "", "", err
		}
		return gitHash, string(commitLog), nil
	}
}
