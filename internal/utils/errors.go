package utils

import "fmt"

// argError denotes that a function's argument was passed incorrectly.
type argError struct {
	message string
}

func (e argError) Error() string {
	return fmt.Sprintf("Argument error: %s", e.message)
}

type missingFileError struct {
	FilePath string
}

func (e missingFileError) Error() string {
	return fmt.Sprintf("file %s not found", e.FilePath)
}
