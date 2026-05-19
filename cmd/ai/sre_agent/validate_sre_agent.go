package sreagent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/openshift/osdctl/internal/utils"
)

// validateSreAgent checks if sre-agent is installed
func validateSreAgent() bool {
	baseDir := filepath.Join(xdg.DataHome, "sre-agent")
	venvBinary := filepath.Join(baseDir, "venv/bin/sre-agent")

	// Check if sre-agent binary exists
	if utils.FileExists(venvBinary) {
		return true // Already installed
	}

	fmt.Fprintf(os.Stderr, "sre-agent is not found in %s\n\n", venvBinary)

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
		fmt.Fprintf(os.Stderr, "Failed to create base directory: %v\n", err)
		return false
	}

	// Copy venv to XDG data directory
	venvPath := filepath.Join(baseDir, "venv")
	if err := copyRepository(userVenvPath, venvPath); err != nil {
		fmt.Fprintf(os.Stderr, "\nCopy failed: %v\n", err)
		return false
	}

	fmt.Fprintln(os.Stderr, "\n✓ sre-agent venv copied successfully")
	return true
}
