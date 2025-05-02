package git

import (
	"strings"
	"sync"

	"github.com/openshift/osdctl/cmd/promote/iexec"
)

var (
	baseDirOnce sync.Once
	BaseDir     string
	baseDirErr  error
)

// getBaseDir returns the base directory of the git repository, this can only be called once per process
func getBaseDir() (string, error) {
	baseDirOnce.Do(func() {
		exec := iexec.Exec{}
		baseDirOutput, err := exec.Output("git", "rev-parse", "--show-toplevel")
		if err != nil {
			baseDirErr = err
			return
		}

		BaseDir = strings.TrimSpace(string(baseDirOutput))
	})

	return BaseDir, baseDirErr
}

/*
// Not in use
func checkBehindMaster(dir string) error {
	fmt.Printf("### Checking 'master' branch is up to date ###\n")

	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error executing git rev-parse command: %v", err)
	}

	branch := strings.TrimSpace(string(output))
	if branch != "master" {
		return fmt.Errorf("you are not on the 'master' branch")
	}

	// Fetch the latest changes from the upstream repository
	cmd = exec.Command("git", "fetch", "upstream")
	cmd.Dir = dir
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error executing git fetch command: %v", err)
	}

	cmd = exec.Command("git", "rev-list", "--count", "HEAD..upstream/master")
	cmd.Dir = dir
	output, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("error executing git rev-list command: %v", err)
	}

	behindCount := strings.TrimSpace(string(output))
	if behindCount != "0" {
		return fmt.Errorf("you are behind 'master' by this many commits: %s", behindCount)
	}
	fmt.Printf("### 'master' branch is up to date ###\n\n")

	return nil
}
*/
