package config

import (
	"io/ioutil"
	"log"
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	LoginScripts map[string]string `yaml:"loginScripts"`
}

func Load() Config {
	config := Config{
		LoginScripts: map[string]string{},
	}
	configFilePath := os.Getenv("HOME") + "/.osdctl.yaml"
	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		return config
	}
	yamlFile, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Printf("Failed to read config yaml %s: %v ", configFilePath, err)
	}
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}

	return config
}
