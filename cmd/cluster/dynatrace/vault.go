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

func getSecretFromVault(vaultAddr, vaultPath string) (id string, secret string, error error) {
	err := os.Setenv("VAULT_ADDR", vaultAddr)
	if err != nil {
		fmt.Printf("Error setting environment variable: %v\n", err)
		return "", "", err
	}
	cmd := exec.Command("vault", "login", "-method=oidc", "-no-print")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err = cmd.Run(); err != nil {
		fmt.Println("Error running 'vault login':", err)
		return "", "", nil
	}

	err = os.Setenv("VAULT_ADDR", vaultAddr)
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
