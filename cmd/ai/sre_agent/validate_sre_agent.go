package sreagent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/openshift/osdctl/internal/utils"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// validateSreAgent checks if sre-agent is installed
func validateSreAgent(homeDir string) bool {
	baseDir := filepath.Join(homeDir, ".local/share/sre-agent")
	venvBinary := filepath.Join(baseDir, "venv/bin/sre-agent")

	// Check if sre-agent binary exists
	if utils.FileExists(venvBinary) {
		return true // Already installed
	}

	fmt.Fprintf(os.Stderr, "sre-agent is not found in ~/.local/share/sre-agent/venv/\n\n")

	// Ask for path to sre-agent venv
	fmt.Fprint(os.Stderr, "Enter the absolute path to sre-agent venv directory: ")
	userVenvPath, err := promptUserInput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read input: %v\n", err)
		return false
	}

	// Validate venv binary exists in provided path
	userVenvBinary := filepath.Join(userVenvPath, "bin/sre-agent")
	if !utils.FileExists(userVenvBinary) {
		fmt.Fprintln(os.Stderr, "\nsre-agent isn't installed")
		return false
	}

	// Create base directory
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		cmdutil.CheckErr(fmt.Errorf("failed to create base directory: %w", err))
	}

	// Copy venv to ~/.local/share/sre-agent/venv
	venvPath := filepath.Join(baseDir, "venv")
	if err := copyRepository(userVenvPath, venvPath); err != nil {
		fmt.Fprintf(os.Stderr, "\nCopy failed: %v\n", err)
		return false
	}

	fmt.Fprintln(os.Stderr, "\n✓ sre-agent venv copied successfully")
	return true
}
