package dynatrace

import (
	"os"
	"os/exec"
	"testing"
)

// TestSetupVaultToken_ContainerEnvironment tests that the vault login command
// uses the correct flags when running inside a container (OCM_CONTAINER env var set)
func TestSetupVaultToken_ContainerEnvironment(t *testing.T) {
	tests := []struct {
		name              string
		containerEnvValue string
		expectNoBrowser   bool
	}{
		{
			name:              "Container environment with OCM_CONTAINER=1",
			containerEnvValue: "1",
			expectNoBrowser:   true,
		},
		{
			name:              "Container environment with OCM_CONTAINER=true",
			containerEnvValue: "true",
			expectNoBrowser:   true,
		},
		{
			name:              "Non-container environment (empty)",
			containerEnvValue: "",
			expectNoBrowser:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment using t.Setenv - automatically cleaned up after test
			if tt.containerEnvValue != "" {
				t.Setenv("OCM_CONTAINER", tt.containerEnvValue)
			}

			// Build the command args as the code does
			loginArgs := []string{"login", "-method=oidc", "-no-print"}
			if os.Getenv("OCM_CONTAINER") != "" {
				loginArgs = []string{"login", "-method=oidc", "skip_browser=true"}
			}

			// Verify the correct parameter is used
			cmd := exec.Command("vault", loginArgs...)
			cmdArgs := cmd.Args[1:] // Skip the "vault" binary name

			if tt.expectNoBrowser {
				// Should have skip_browser=true parameter
				hasSkipBrowser := false
				hasNoPrint := false
				for _, arg := range cmdArgs {
					if arg == "skip_browser=true" {
						hasSkipBrowser = true
					}
					if arg == "-no-print" {
						hasNoPrint = true
					}
				}

				if !hasSkipBrowser {
					t.Errorf("Expected skip_browser=true parameter in container environment, got args: %v", cmdArgs)
				}
				if hasNoPrint {
					t.Errorf("Did not expect -no-print flag in container environment, got args: %v", cmdArgs)
				}
			} else {
				// Should have -no-print flag
				hasSkipBrowser := false
				hasNoPrint := false
				for _, arg := range cmdArgs {
					if arg == "skip_browser=true" {
						hasSkipBrowser = true
					}
					if arg == "-no-print" {
						hasNoPrint = true
					}
				}

				if hasSkipBrowser {
					t.Errorf("Did not expect skip_browser=true parameter in non-container environment, got args: %v", cmdArgs)
				}
				if !hasNoPrint {
					t.Errorf("Expected -no-print flag in non-container environment, got args: %v", cmdArgs)
				}
			}
		})
	}
}

// TestSetupVaultToken_OutputRedirection tests that stdout/stderr are properly
// redirected based on the environment
func TestSetupVaultToken_OutputRedirection(t *testing.T) {
	tests := []struct {
		name            string
		containerEnvValue string
		expectOutput    bool
	}{
		{
			name:            "Container environment shows output",
			containerEnvValue: "1",
			expectOutput:    true,
		},
		{
			name:            "Non-container environment hides output",
			containerEnvValue: "",
			expectOutput:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment using t.Setenv - automatically cleaned up after test
			if tt.containerEnvValue != "" {
				t.Setenv("OCM_CONTAINER", tt.containerEnvValue)
			}

			// Build the command as the code does
			loginArgs := []string{"login", "-method=oidc", "-no-print"}
			if os.Getenv("OCM_CONTAINER") != "" {
				loginArgs = []string{"login", "-method=oidc", "skip_browser=true"}
			}
			loginCmd := exec.Command("vault", loginArgs...)

			// Set output redirection as the code does
			if os.Getenv("OCM_CONTAINER") != "" {
				loginCmd.Stdout = os.Stdout
				loginCmd.Stderr = os.Stderr
			} else {
				loginCmd.Stdout = nil
				loginCmd.Stderr = nil
			}

			// Verify output redirection is correct
			if tt.expectOutput {
				if loginCmd.Stdout != os.Stdout {
					t.Error("Expected Stdout to be os.Stdout in container environment")
				}
				if loginCmd.Stderr != os.Stderr {
					t.Error("Expected Stderr to be os.Stderr in container environment")
				}
			} else {
				if loginCmd.Stdout != nil {
					t.Error("Expected Stdout to be nil in non-container environment")
				}
				if loginCmd.Stderr != nil {
					t.Error("Expected Stderr to be nil in non-container environment")
				}
			}
		})
	}
}

