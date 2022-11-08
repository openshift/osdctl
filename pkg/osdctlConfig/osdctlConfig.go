package osdctlConfig

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/viper"
)

const (
	ConfigFileName = "osdctl"
)

func EnsureConfigFile() error {
	configPath, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configFilePath := configPath + "/.config/" + ConfigFileName
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
	} else {
		fmt.Println("Reading config file from " + configFilePath)
	}
	return nil
}
