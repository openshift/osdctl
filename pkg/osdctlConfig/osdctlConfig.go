package osdctlConfig

import (
	"errors"
	"os"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

const (
	ConfigFileName = "osdctl"
	ConfifFilePath = "~/.config"
)

// Generates the config file path for osdctl config
func generateConfigFilePath() (string, error) {
	configFilePath, err := xdg.ConfigFile(ConfigFileName)
	if err != nil {
		return "", err
	}
	return configFilePath, nil
}

func EnsureConfigFile() error {
	configFilePath, err := generateConfigFilePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(configFilePath); errors.Is(err, os.ErrNotExist) {
		_, err = os.Create(configFilePath)
		if err != nil {
			return err
		}
	}

	viper.SetConfigFile(configFilePath)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return err
	}

	return err
}
