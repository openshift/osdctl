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
		err = os.MkdirAll(configFileDir, 0750)
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

// GetConfigValues reads the osdctl config file using a dedicated viper instance,
// avoiding the global viper which backplane-cli overwrites concurrently.
// TODO: Remove this workaround once backplane-cli stops overwriting the global viper instance.
func GetConfigValues(keys ...string) (map[string]string, error) {
	configHomePath, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	configFilePath := configHomePath + "/.config/" + ConfigFileName

	v := viper.New()
	v.SetConfigFile(configFilePath)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	values := make(map[string]string, len(keys))
	for _, k := range keys {
		values[k] = v.GetString(k)
	}
	return values, nil
}
