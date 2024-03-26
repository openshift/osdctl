package logs

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

type VaultResponse struct {
	Data struct {
		Data map[string]interface{} `json:"data"`
	} `json:"data"`
}

func getSecretFromVault() (string, error) {
	err := os.Setenv("VAULT_ADDR", vaultAddr)
	if err != nil {
		fmt.Printf("Error setting environment variable: %v\n", err)
		return "", err
	}
	loginCommand := exec.Command("vault", "login", "-method=oidc", "-no-print")
	loginCommand.Stdout = nil
	loginCommand.Stderr = nil
	if err := loginCommand.Run(); err != nil {
		fmt.Println("Error running 'vault login':", err)
		return "", nil
	}

	err = os.Setenv("VAULT_ADDR", vaultAddr)
	if err != nil {
		fmt.Printf("Error setting environment variable: %v\n", err)
		return "", err
	}
	kvGetCommand := exec.Command("vault", "kv", "get", "-format=json", vaultPath)
	output, err := kvGetCommand.Output()
	if err != nil {
		fmt.Println("Error running 'vault kv get':", err)
		return "", nil
	}

	var vaultResponse VaultResponse
	if err := json.Unmarshal(output, &vaultResponse); err != nil {
		fmt.Printf("Error unmarshaling JSON response: %v\n", err)
		os.Exit(1)
	}
	secretData, ok := vaultResponse.Data.Data["dt0s02.5QMWDNHL"].(string)
	if !ok {
		fmt.Println("Error extracting secret data from JSON response")
		os.Exit(1)
	}
	return secretData, nil
}
