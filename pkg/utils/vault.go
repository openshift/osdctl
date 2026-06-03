package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	ocmutils "github.com/openshift/ocm-container/pkg/utils"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	vaultCallbackPortFile = "/tmp/vault_callback_port"
	defaultVaultOIDCPort  = "8250"
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
	if vaultAddr == "" {
		return VaultRef{}, fmt.Errorf("vault address set in config file is empty")
	}

	if !viper.IsSet(vaultPathKey) {
		return VaultRef{}, fmt.Errorf("key '%s' is not set in config file", vaultPathKey)
	}
	vaultPath := viper.GetString(vaultPathKey)
	if vaultPath == "" {
		return VaultRef{}, fmt.Errorf("vault path set in config file is empty")
	}

	return VaultRef{Addr: vaultAddr, Path: vaultPath}, nil
}

// readCallbackPort reads the dynamically-assigned host port from the
// portmap file written by ocm-container's ports feature.
func readCallbackPort() string {
	data, err := os.ReadFile(vaultCallbackPortFile)
	if err != nil {
		log.Debugf("could not read vault callback port file: %v", err)
		return ""
	}
	port := strings.TrimSpace(string(data))
	if port == "" {
		log.Debugf("vault callback port file is empty")
		return ""
	}
	log.Debugf("read vault callback port: %s", port)
	return port
}

// setupVaultToken ensures a valid Vault token exists by checking the current
// token and requesting a new one via OIDC if needed. In container environments,
// it uses the dynamically-assigned callback port from ocm-container's ports
// feature and falls back to in-process token capture if the token file is
// not writable.
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
	if err = tokenCheckCmd.Run(); err != nil {
		log.Infoln("Vault token no longer valid, requesting new token")

		if ocmutils.IsRunningInOcmContainer() {
			return setupVaultTokenContainer()
		}

		return setupVaultTokenLocal()
	}

	return nil
}

// setupVaultTokenLocal handles vault OIDC login outside of a container.
func setupVaultTokenLocal() error {
	loginCmd := exec.Command("vault", "login", "-method=oidc", "-no-print")
	loginCmd.Stdout = nil
	loginCmd.Stderr = nil

	if err := loginCmd.Run(); err != nil {
		return fmt.Errorf("error running 'vault login': %v", err)
	}

	log.Infoln("Acquired vault token")
	return nil
}

// containerOIDCArgs builds the vault login args for container mode,
// including the dynamic callback port if available.
func containerOIDCArgs(noStore bool) []string {
	callbackPort := readCallbackPort()

	args := []string{"login", "-method=oidc"}
	if noStore {
		args = append(args, "-no-store", "-field=token")
	}
	args = append(args, "skip_browser=true", "listenaddress=0.0.0.0")

	if callbackPort != "" {
		args = append(args,
			fmt.Sprintf("port=%s", defaultVaultOIDCPort),
			fmt.Sprintf("callbackport=%s", callbackPort),
		)
		log.Infof("Using dynamic vault OIDC callback port: %s", callbackPort)
	} else {
		log.Infoln("No dynamic callback port found, using default port 8250.")
		log.Infoln("If running multiple containers, ensure ocm-container has the ports feature enabled.")
	}

	return args
}

// setupVaultTokenContainer handles vault OIDC login inside an ocm-container.
// It first tries a normal vault login that writes ~/.vault-token. If that
// fails (e.g., read-only bind mount), it falls back to capturing the token
// in-process via VAULT_TOKEN environment variable.
func setupVaultTokenContainer() error {
	log.Infoln("Complete the login via the URL printed below.")

	// First attempt: normal login that writes ~/.vault-token
	loginArgs := containerOIDCArgs(false)
	loginCmd := exec.Command("vault", loginArgs...)
	loginCmd.Stdout = os.Stderr
	loginCmd.Stderr = os.Stderr

	if err := loginCmd.Run(); err == nil {
		log.Infoln("Acquired vault token")
		return nil
	}

	// If the normal login failed, it may be because ~/.vault-token is a
	// read-only bind mount or has rename issues. Fall back to capturing
	// the token in-process.
	log.Infoln("Token file write failed, capturing token in-process instead.")
	log.Infoln("Complete the login via the URL printed below.")

	loginArgs = containerOIDCArgs(true)
	loginCmd = exec.Command("vault", loginArgs...)

	var tokenBuf bytes.Buffer
	loginCmd.Stdout = &tokenBuf
	loginCmd.Stderr = os.Stderr

	if err := loginCmd.Run(); err != nil {
		return fmt.Errorf("vault login failed: %v\n\n"+
			"If authentication timed out or the callback failed:\n"+
			"  1. Ensure ocm-container has the vault port enabled in the ports feature\n"+
			"  2. Restart your ocm-container for the change to take effect\n"+
			"  3. Try the authentication again", err)
	}

	token := strings.TrimSpace(tokenBuf.String())
	if token == "" {
		return fmt.Errorf("vault login succeeded but returned empty token")
	}

	if err := os.Setenv("VAULT_TOKEN", token); err != nil {
		return fmt.Errorf("error setting VAULT_TOKEN: %v", err)
	}

	log.Infoln("Acquired vault token (in-process)")
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

	kvGetCommand := exec.Command("vault", "kv", "get", "-format=json", vaultRef.Path) //nolint:gosec
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
