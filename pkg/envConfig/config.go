package config

import (
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
	CloudTrailList struct {
		FilterPatternList []string `mapstructure:"filter_regex_patterns"`
	} `mapstructure:"cloudtrail_cmd_lists"`
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

// Loads ~/.config/osdctl
func LoadCloudTrailConfig() ([]string, error) {
	var configuration *CloudTrailConfig
	err := osdctlConfig.EnsureConfigFile()
	if err != nil {
		return nil, err
	}

	err = viper.Unmarshal(&configuration)
	if err != nil {
		log.Printf("[ERROR] Failed to unmarshal Cloudtrail config yaml %s %v", viper.ConfigFileUsed(), err)
		return nil, err
	}

	return configuration.CloudTrailList.FilterPatternList, err
}
