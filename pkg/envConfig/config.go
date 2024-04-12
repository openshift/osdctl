package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/openshift/osdctl/pkg/osdctlConfig"
	"github.com/spf13/viper"
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

// cloudtrailCmd configuration struct for parsing configuration options
type CloudTrailConfig struct {
	Cloudtrail_list struct {
		FilterPatternList []string `mapstructure:"cloudtrail_cmd_lists"`
	} `mapstructure:"filter_regex_patterns"`
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
	yamlFile, err := os.ReadFile(configFilePath) //#nosec G304 -- filepath cannot be constant
	if err != nil {
		log.Printf("Failed to read config yaml %s: %v ", configFilePath, err)
	}
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Failed to unmarshal config yaml %s: %v", configFilePath, err)
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
	jsonFile, err := os.ReadFile(configFilePath) //#nosec G304 -- filepath cannot be constant
	if err != nil {
		log.Printf("Failed to read PagerDuty config json %s: %v ", configFilePath, err)
	}

	err = json.Unmarshal(jsonFile, &config)
	if err != nil {
		log.Fatalf("Failed to unmarshal PagerDuty config json %s: %v", configFilePath, err)
	}
	return config
}

// Loads ~/.config/osdctl
func LoadCloudTrailConfig() ([]string, error) {
	var configuration *CloudTrailConfig
	osdctlConfig.EnsureConfigFile()
	err := viper.Unmarshal(&configuration)
	if err != nil {
		log.Printf("Failed to unmarshal Cloudtrail config yaml %s %v", viper.ConfigFileUsed(), err)
		return nil, err
	}
	osdctlConfig.EnsureConfigFile()

	return configuration.Cloudtrail_list.FilterPatternList, err
}
