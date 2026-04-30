package dynatrace

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	ocmutils "github.com/openshift/ocm-container/pkg/utils"
)

type response struct {
	Data struct {
		Data map[string]interface{} `json:"data"`
	} `json:"data"`
}

// setupVaultToken ensures a valid Vault token exists by checking the current
// token and requesting a new one via OIDC if needed. In container environments,
// it configures authentication to work without browser auto-launch.
func setupVaultToken(vaultAddr string) error {
	err := os.Setenv("VAULT_ADDR", vaultAddr)
	if err != nil {
		return fmt.Errorf("error setting environment variable: %v", err)
	}

	versionCheckCmd := exec.Command("vault", "version")

	versionCheckCmd.Stdout = os.Stdout
	versionCheckCmd.Stderr = os.Stderr

	if err = versionCheckCmd.Run(); err != nil {
		return fmt.Errorf("missing vault cli: %v", err)
	}

	tokenCheckCmd := exec.Command("vault", "token", "lookup")
	tokenCheckCmd.Stdout = nil
	tokenCheckCmd.Stderr = nil
	// get new token since old token has expired
	if err = tokenCheckCmd.Run(); err != nil {
		fmt.Println("Vault token no longer valid, requesting new token")

		// Check if we're in a container environment (OCM_CONTAINER env var is set)
		// If so, skip automatic browser launch and print the URL for manual authentication
		loginArgs := []string{"login", "-method=oidc", "-no-print"}
		if ocmutils.IsRunningInOcmContainer() {
			fmt.Println("\nNOTE: Running in container mode - OIDC authentication requires port forwarding.")
			fmt.Println("Ensure port 8250 is exposed in your ocm-container configuration:")
			fmt.Println("  Add 'launch-opts: \"-p 8250:8250\"' to ~/.config/ocm-container/ocm-container.yaml")
			fmt.Println("Then restart your ocm-container for the change to take effect.")

			// In container: skip browser launch and listen on all interfaces (0.0.0.0)
			// so the callback can be reached from the host browser via localhost:8250
			loginArgs = []string{"login", "-method=oidc", "skip_browser=true", "listenaddress=0.0.0.0"}
		}
		loginCmd := exec.Command("vault", loginArgs...)

		// Show output when using skip_browser so user can see the authentication URL
		if ocmutils.IsRunningInOcmContainer() {
			loginCmd.Stdout = os.Stdout
			loginCmd.Stderr = os.Stderr
		} else {
			loginCmd.Stdout = nil
			loginCmd.Stderr = nil
		}

		if err = loginCmd.Run(); err != nil {
			if ocmutils.IsRunningInOcmContainer() {
				return fmt.Errorf("vault login failed: %v\n\n"+
					"If authentication timed out or the callback failed, this is likely because:\n"+
					"  1. Port 8250 is not exposed in your ocm-container configuration\n"+
					"  2. Your ocm-container was not restarted after adding the port\n\n"+
					"To fix:\n"+
					"  - Add 'launch-opts: \"-p 8250:8250\"' to ~/.config/ocm-container/ocm-container.yaml\n"+
					"  - Exit and restart your ocm-container\n"+
					"  - Try the authentication again", err)
			}
			return fmt.Errorf("error running 'vault login': %v", err)
		}

		fmt.Println("Acquired vault token")
	}

	return nil
}

func getSecretFromVault(vaultAddr, vaultPath string) (id string, secret string, error error) {
	err := os.Setenv("VAULT_ADDR", vaultAddr)
	if err != nil {
		return "", "", fmt.Errorf("error setting environment variable: %v", err)
	}

	kvGetCommand := exec.Command("vault", "kv", "get", "-format=json", vaultPath)
	output, err := kvGetCommand.Output()
	if err != nil {
		fmt.Println("Error running 'vault kv get':", err)
		return "", "", nil
	}

	var resp response
	if err := json.Unmarshal(output, &resp); err != nil {
		return "", "", fmt.Errorf("error unmarshaling JSON response: %v", err)
	}
	clientID, ok := resp.Data.Data["client_id"].(string)
	if !ok {
		return "", "", fmt.Errorf("error extracting secret data from JSON response")
	}
	clientSecret, ok := resp.Data.Data["client_secret"].(string)
	if !ok {
		return "", "", fmt.Errorf("error extracting secret data from JSON response")
	}

	return clientID, clientSecret, nil
}
