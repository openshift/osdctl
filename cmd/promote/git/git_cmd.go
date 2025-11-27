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

// GetBaseDir returns the base directory of the git repository, this can only be called once per process
func GetBaseDir(exec iexec.IExec) (string, error) {
	baseDirOnce.Do(func() {
		baseDirOutput, err := exec.Output("", "git", "rev-parse", "--show-toplevel")
		if err != nil {
			baseDirErr = err
			return
		}

		BaseDir = strings.TrimSpace(baseDirOutput)
	})

	return BaseDir, baseDirErr
}
