package dynatrace

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

func CheckoutAndCompareGitHash(appInterface AppInterface, gitURL, gitHash, currentGitHash, serviceFullPath string) (string, string, error) {
	tempDir, err := ioutil.TempDir("", "")
	exec := appInterface.GitExecutor
	if err != nil {
		return "", "", fmt.Errorf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	err = os.Chdir(tempDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to change directory to temporary directory: %v", err)
	}

	err = exec.Run("", "git", "clone", gitURL, "source-dir")
	if err != nil {
		return "", "", fmt.Errorf("failed to clone git repository: %v", err)
	}

	err = os.Chdir("source-dir")
	if err != nil {
		return "", "", fmt.Errorf("failed to change directory to source-dir: %v", err)
	}

	if gitHash == "" {
		fmt.Printf("No git hash provided. Using HEAD.\n")
		output, err := exec.Output("", "git", "rev-list", "-n", "1", "HEAD", "--", serviceFullPath)
		if err != nil {
			return "", "", fmt.Errorf("failed to get git hash: %v", err)
		}
		gitHash = strings.TrimSpace(string(output))
		fmt.Printf("The head githash is %s\n", gitHash)
	}

	if currentGitHash == gitHash {
		return "", "", fmt.Errorf("git hash %s is already at HEAD", gitHash)
	} else {
		commitLog, err := exec.Output("git", "log", "--no-merges", fmt.Sprintf("%s..%s", currentGitHash, gitHash))
		if err != nil {
			return "", "", err
		}
		return gitHash, string(commitLog), nil
	}
}
