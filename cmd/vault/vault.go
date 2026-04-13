package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	ocmutils "github.com/openshift/ocm-container/pkg/utils"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	VaultAddrKey string = "vault_address"
)

type VaultRef struct {
	Addr string
	Path string
}

func GetVaultRef(vaultPathKey string) (VaultRef, error) {
	if !viper.IsSet(VaultAddrKey) {
		return VaultRef{}, fmt.Errorf("key '%s' is not set in config file", VaultAddrKey)
	}
	vaultAddr := viper.GetString(VaultAddrKey)

	if !viper.IsSet(vaultPathKey) {
		return VaultRef{}, fmt.Errorf("key '%s' is not set in config file", vaultPathKey)
	}
	vaultPath := viper.GetString(vaultPathKey)

	return VaultRef{Addr: vaultAddr, Path: vaultPath}, nil
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

	versionCheckCmd.Stdout = os.Stderr
	versionCheckCmd.Stderr = os.Stderr

	if err = versionCheckCmd.Run(); err != nil {
		return fmt.Errorf("missing vault cli: %v", err)
	}

	tokenCheckCmd := exec.Command("vault", "token", "lookup")
	tokenCheckCmd.Stdout = nil
	tokenCheckCmd.Stderr = nil
	// get new token since old token has expired
	if err = tokenCheckCmd.Run(); err != nil {
		log.Infoln("Vault token no longer valid, requesting new token")

		// Check if we're in a container environment (OCM_CONTAINER env var is set)
		// If so, skip automatic browser launch and print the URL for manual authentication
		loginArgs := []string{"login", "-method=oidc", "-no-print"}
		if ocmutils.IsRunningInOcmContainer() {
			log.Infoln("\nNOTE: Running in container mode - OIDC authentication requires port forwarding.")
			log.Infoln("Ensure port 8250 is exposed in your ocm-container configuration:")
			log.Infoln("  Add 'launch-opts: \"-p 8250:8250\"' to ~/.config/ocm-container/ocm-container.yaml")
			log.Infoln("Then restart your ocm-container for the change to take effect.")

			// In container: skip browser launch and listen on all interfaces (0.0.0.0)
			// so the callback can be reached from the host browser via localhost:8250
			loginArgs = []string{"login", "-method=oidc", "skip_browser=true", "listenaddress=0.0.0.0"}
		}
		loginCmd := exec.Command("vault", loginArgs...)

		// Show output when using skip_browser so user can see the authentication URL
		if ocmutils.IsRunningInOcmContainer() {
			loginCmd.Stdout = os.Stderr
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

		log.Infoln("Acquired vault token")
	}

	return nil
}

type vaultOutput struct {
	Data struct {
		Data map[string]string `json:"data"`
	} `json:"data"`
}

func GetSecretFromVault(vaultRef VaultRef) (*map[string]string, error) {
	err := setupVaultToken(vaultRef.Addr)
	if err != nil {
		return nil, err
	}

	err = os.Setenv("VAULT_ADDR", vaultRef.Addr)
	if err != nil {
		return nil, fmt.Errorf("error setting environment variable: %v", err)
	}

	kvGetCommand := exec.Command("vault", "kv", "get", "-format=json", vaultRef.Path)
	output, err := kvGetCommand.Output()
	if err != nil {
		return nil, fmt.Errorf("error running 'vault kv get %s' (VAULT_ADDR: %s): %v", vaultRef.Path, vaultRef.Addr, err)
	}

	var formattedOutput vaultOutput
	if err := json.Unmarshal(output, &formattedOutput); err != nil {
		return nil, fmt.Errorf("error unmarshaling JSON output: %v", err)
	}

	return &formattedOutput.Data.Data, nil
}

func GetCredsFromVault(configKey string) (string, string, error) {
	vaultRef, err := GetVaultRef(configKey)
	if err != nil {
		return "", "", err
	}

	secretData, err := GetSecretFromVault(vaultRef)
	if err != nil {
		return "", "", err
	}

	clientID, ok := (*secretData)["client_id"]
	if !ok {
		return "", "", fmt.Errorf("no 'client_id' in %s vault secret (VAULT_ADDR: %s)", vaultRef.Path, vaultRef.Addr)
	}
	clientSecret, ok := (*secretData)["client_secret"]
	if !ok {
		return "", "", fmt.Errorf("no 'client_secret' in %s vault secret (VAULT_ADDR: %s)", vaultRef.Path, vaultRef.Addr)
	}

	return clientID, clientSecret, nil
}
