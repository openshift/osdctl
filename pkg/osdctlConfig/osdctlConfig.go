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
	configFilePath := configHomePath + "/.config/" + ConfigFileName
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
	return nil
}
