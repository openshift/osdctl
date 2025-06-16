package git

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/openshift/osdctl/cmd/promote/iexec"
)

func CheckoutAndCompareGitHash(gitExecutor iexec.IExec, gitURL, gitHash, currentGitHash string) (string, string, error) {
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	err = os.Chdir(tempDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to change directory to temporary directory: %v", err)
	}

	err = gitExecutor.Run(tempDir, "git", "clone", gitURL, "source-dir")
	if err != nil {
		return "", "", fmt.Errorf("failed to clone git repository: %v", err)
	}
	err = os.Chdir("source-dir")
	if err != nil {
		return "", "", fmt.Errorf("failed to change directory to source-dir: %v", err)
	}

	if gitHash == "" {
		fmt.Printf("No git hash provided. Using HEAD.\n")
		output, err := gitExecutor.Output("", "git", "rev-parse", "HEAD")
		if err != nil {
			return "", "", fmt.Errorf("failed to get git hash: %v", err)
		}
		gitHash = strings.TrimSpace(output)
		fmt.Printf("The head githash is %s\n", gitHash)
	}

	if currentGitHash == gitHash {
		return "", "", fmt.Errorf("git hash %s is already at HEAD", gitHash)
	} else {
		commitLog, err := gitExecutor.Output("", "git", "log", "--no-merges", fmt.Sprintf("%s..%s", currentGitHash, gitHash))
		if err != nil {
			return "", "", err
		}
		return gitHash, commitLog, nil
	}
}
