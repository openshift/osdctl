package utils

import (
	"os"

	log "github.com/sirupsen/logrus"
)

// GetEnv returns the value of the environment variable specified by key, if the variable is set.
func GetEnv(key string) (string, error) {
	if key == "" {
		log.Debug("Key is empty")
		return "", argError{"empty argument"}
	}

	val, ok := os.LookupEnv(key)
	if !ok {
		return "", envVarError{key, "environment variable not found"}
	}

	if val == "" {
		return "", envVarError{key, "environment variable is empty"}
	}

	return val, nil
}
