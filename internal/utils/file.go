package utils

import (
	"fmt"
	"net/url"
	"os"

	osFile "path/filepath"

	log "github.com/sirupsen/logrus"
)

// Exists reports whether the named file or directory exists.
func exists(path string, isDir bool) bool {
	if path == "" {
		log.Debug("Path is empty")
		return false
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return false
		}
	}

	return isDir == info.IsDir()
}

// FolderExists reports whether the provided directory exists.
func FolderExists(path string) bool {
	return exists(path, true)
}

// FileExists reports whether the provided file exists.
func FileExists(path string) bool {
	return exists(path, false)
}

// AccessFile returns the contents of a local file or URL.
func AccessFile(filePath string) ([]byte, error) {
	if IsValidUrl(filePath) {
		urlPage, _ := url.Parse(filePath)
		if err := IsOnline(*urlPage); err != nil {
			return nil, fmt.Errorf("host %q is not accessible", filePath)
		}
		return CurlThis(urlPage.String())
	}

	filePath = osFile.Clean(filePath)
	if FileExists(filePath) {
		file, err := os.ReadFile(filePath) //#nosec G304 -- Potential file inclusion via variable
		if err != nil {
			return file, fmt.Errorf("cannot read the file: %w", err)
		}
		return file, nil
	}
	if FolderExists(filePath) {
		return nil, fmt.Errorf("the provided path %q is a directory, not a file", filePath)
	}
	return nil, fmt.Errorf("cannot read the file %q", filePath)
}

// CreateFile creates a file on the given filepath
// along with any necessary parents, and returns nil,
// or else returns an error.
func CreateFile(filepath string) error {
	filepath = osFile.Clean(filepath)
	// Avoid file truncate and return error instead
	if FileExists(filepath) {
		return fmt.Errorf("file %s already exists", filepath)
	}

	// Create the parent directory if doesn't exist
	if directory := osFile.Dir(filepath); !FolderExists(directory) {
		if err := os.MkdirAll(directory, 0750); err != nil {
			return fmt.Errorf("failed to create directory %v", directory)
		}
	}
	file, err := os.Create(filepath) //#nosec G304 -- ignore potential file inclusion via variable
	if err != nil {
		return fmt.Errorf("failed to create file %v: %w", filepath, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Println("Error closing file", filepath)
		}
	}()

	return nil
}
