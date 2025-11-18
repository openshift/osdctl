package git

import (
	"fmt"
	"os"
	"strings"

	"github.com/openshift/osdctl/cmd/promote/iexec"
)

func CheckoutAndCompareGitHash(gitExecutor iexec.IExec, gitURL, gitHash, currentGitHash string, serviceFullPath ...string) (string, string, error) {
	tempDir, err := os.MkdirTemp("", "")
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
		var output string
		// If serviceFullPath is provided, get the latest commit for that specific path
		if len(serviceFullPath) > 0 && serviceFullPath[0] != "" {
			output, err = gitExecutor.Output("", "git", "rev-list", "-n", "1", "HEAD", "--", serviceFullPath[0])
		} else {
			output, err = gitExecutor.Output("", "git", "rev-parse", "HEAD")
		}
		if err != nil {
			return "", "", fmt.Errorf("failed to get git hash: %v", err)
		}
		gitHash = strings.TrimSpace(output)
		fmt.Printf("The head githash is %s\n", gitHash)
	}

	if currentGitHash == gitHash {
		return "", "", fmt.Errorf("git hash %s is already at HEAD", gitHash)
	} else {
		var commitLog string
		var err error
		// If serviceFullPAth is provided, filter logs to only show changes in that path
		if len(serviceFullPath) > 0 && serviceFullPath[0] != "" {
			commitLog, err = gitExecutor.Output("", "git", "log", "--no-merges", fmt.Sprintf("%s..%s", currentGitHash, gitHash), "--", serviceFullPath[0])
		} else {
			commitLog, err = gitExecutor.Output("", "git", "log", "--no-merges", fmt.Sprintf("%s..%s", currentGitHash, gitHash))
		}
		if err != nil {
			return "", "", err
		}
		return gitHash, commitLog, nil
	}
}
