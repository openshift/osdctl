package config

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

type Config struct {
	LoginScripts map[string]string `yaml:"loginScripts"`
}

type Subdomain struct {
	AccessToken string `json:"accessToken"`
}

type PDConfig struct {
	MySubdomain []Subdomain `json:"subdomains"`
}

func LoadYaml(paramFilePath string) Config {
	config := Config{
		LoginScripts: map[string]string{},
	}
	configFilePath := os.Getenv("HOME") + paramFilePath
	configFilePath = filepath.Clean(configFilePath)
	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		return config
	}
	// ignore linter error: filepath has to be static
	yamlFile, err := ioutil.ReadFile(configFilePath) //#nosec G304 -- filepath cannot be constant
	if err != nil {
		log.Printf("Failed to read config yaml %s: %v ", configFilePath, err)
	}
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}

	return config
}

func LoadPDConfig(paramFilePath string) PDConfig {
	config := PDConfig{
		MySubdomain: []Subdomain{},
	}

	configFilePath := os.Getenv("HOME") + paramFilePath
	configFilePath = filepath.Clean(configFilePath)
	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		log.Println("Config does not exist")
		return config
	}

	// ignore linter error: filepath has to be static
	jsonFile, err := ioutil.ReadFile(configFilePath) //#nosec G304 -- filepath cannot be constant
	if err != nil {
		log.Printf("Failed to read config json %s: %v ", configFilePath, err)
	}

	err = json.Unmarshal(jsonFile, &config)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
	return config
}
