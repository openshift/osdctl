package osdctlConfig

import (
	"errors"
	"os"

	"github.com/spf13/viper"
)

const (
	ConfigFileName = "osdctl"
)

func EnsureConfigFile() error {
	configHomePath, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configFileDir := configHomePath + "/.config/"
	configFilePath := configFileDir + ConfigFileName
	if _, err := os.Stat(configFilePath); errors.Is(err, os.ErrNotExist) {
		err = os.MkdirAll(configFileDir, 0755)
		if err != nil {
			return err
		}
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
	return nil
}
