package utils

import "fmt"

// argError denotes that a function's argument was passed incorrectly.
type argError struct {
	message string
}

func (e argError) Error() string {
	return fmt.Sprintf("Argument error: %s", e.message)
}

// envVarError tracks environment variable errors.
type envVarError struct {
	key     string
	message string
}

func (e envVarError) Error() string {
	return fmt.Sprintf("Env variable error: %s: %s", e.message, e.key)
}

type missingFileError struct {
	FilePath string
}

func (e missingFileError) Error() string {
	return fmt.Sprintf("file %s not found", e.FilePath)
}
