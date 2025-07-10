package dynatrace

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

type response struct {
	Data struct {
		Data map[string]interface{} `json:"data"`
	} `json:"data"`
}

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

		loginCmd := exec.Command("vault", "login", "-method=oidc", "-no-print")
		loginCmd.Stdout = nil
		loginCmd.Stderr = nil
		if err = loginCmd.Run(); err != nil {
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
