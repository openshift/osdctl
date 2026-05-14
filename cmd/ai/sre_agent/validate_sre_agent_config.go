package sreagent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/openshift/osdctl/internal/utils"
	"gopkg.in/yaml.v3"
)

// checkSreAgentConfig validates config.yaml and updates ops-sop path if needed
func checkSreAgentConfig(homeDir string) bool {
	baseDir := filepath.Join(homeDir, ".local/share/sre-agent")
	configPath := filepath.Join(homeDir, ".config/sre-agent/config.yaml")

	// Check if config exists
	if !utils.FileExists(configPath) {
		fmt.Fprintln(os.Stderr, "\nsre-agent not configured")
		fmt.Fprintln(os.Stderr, "Config file not found at:", configPath)
		return false
	}

	// Read existing config
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read config: %v\n", err)
		return false
	}

	// Parse YAML
	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse config: %v\n", err)
		return false
	}

	// Get current sop directory from config
	sop, ok := config["sop"].(map[string]interface{})
	if !ok {
		fmt.Fprintln(os.Stderr, "Invalid config: sop section not found")
		return false
	}

	currentSopDir, ok := sop["directory"].(string)
	if !ok {
		fmt.Fprintln(os.Stderr, "Invalid config: sop directory is not a string")
		return false
	}

	// Ask user for ops-sop repository path
	fmt.Fprintln(os.Stderr, "\nChecking ops-sop repository...")
	fmt.Fprint(os.Stderr, "Enter the absolute path to ops-sop repository: ")
	userOpsSopPath, err := promptUserInput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read input: %v\n", err)
		return false
	}

	// Validate path exists
	if !utils.FolderExists(userOpsSopPath) {
		fmt.Fprintln(os.Stderr, "\nThe provided ops-sop path does not exist.")
		return false
	}

	opsSopPath := filepath.Join(baseDir, "ops-sop")

	// Copy ops-sop if not present
	if !utils.FolderExists(opsSopPath) {
		if err := copyRepository(userOpsSopPath, opsSopPath); err != nil {
			fmt.Fprintf(os.Stderr, "\nCopy failed: %v\n", err)
			return false
		}
		fmt.Fprintln(os.Stderr, "✓ ops-sop copied successfully")
	} else {
		fmt.Fprintln(os.Stderr, "✓ ops-sop repository found")
	}

	// Check if sop directory in config is different from expected
	if currentSopDir != opsSopPath {
		// Update config with new path
		sop["directory"] = opsSopPath

		// Write updated config
		updatedData, err := yaml.Marshal(config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to marshal config: %v\n", err)
			return false
		}

		if err := os.WriteFile(configPath, updatedData, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write config: %v\n", err)
			return false
		}

		fmt.Fprintf(os.Stderr, "✓ ops-sop path updated in config: %s\n\n", opsSopPath)
	} else {
		fmt.Fprintf(os.Stderr, "✓ ops-sop path is correct: %s\n\n", opsSopPath)
	}

	return true
}
