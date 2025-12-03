package pathutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DeriveAppYmlPath determines the location of app.yml for a given service.
func DeriveAppYmlPath(gitDirectory, saasFile, componentName string) (string, error) {
	derivedPath, err := deriveFromSaasPath(gitDirectory, saasFile, componentName)
	if err == nil {
		if _, statErr := os.Stat(derivedPath); statErr == nil {
			return derivedPath, nil
		}
	}

	// as fallback, searches filesystem for likely loccations
	searchedPath, searchErr := searchFilesystem(gitDirectory, componentName)
	if searchErr == nil {
		return searchedPath, nil
	}

	if err == nil {
		return derivedPath, nil
	}

	return "", fmt.Errorf("could not determine app.yml path for %s: derive error: %v, search error: %v",
		componentName, err, searchErr)
}

// deriveFromSaasPath derives the app.yml path by parsing the saasFile path structure.
func deriveFromSaasPath(gitDirectory, saasFile, componentName string) (string, error) {
	// absolute saasFile path into relative path
	relPath, err := filepath.Rel(gitDirectory, saasFile)
	if err != nil {
		return "", fmt.Errorf("failed to get relative path from %s to %s: %v", gitDirectory, saasFile, err)
	}

	pathParts := strings.Split(relPath, string(filepath.Separator))
	var parentServiceDir string

	// Find "services" in the path and extract the next component
	for i, part := range pathParts {
		if part == "services" && i+1 < len(pathParts) {
			parentServiceDir = pathParts[i+1]
			break
		}
	}

	if parentServiceDir == "" {
		return "", fmt.Errorf("invalid saasFile path: 'services' directory not found in %s", relPath)
	}

	if parentServiceDir != componentName && parentServiceDir != "cicd" {
		// Nested structure: data/services/{parent}/{component}/app.yml
		return filepath.Join(gitDirectory, "data", "services", parentServiceDir, componentName, "app.yml"), nil
	}

	// Flat structure; data/services/{component}/app.yml
	return filepath.Join(gitDirectory, "data", "services", componentName, "app.yml"), nil
}

// searchFilesystem searches for app.yml in specific "likely" paths
func searchFilesystem(gitDirectory, componentName string) (string, error) {
	parentDirs := []string{
		"",              // services/{component}
		"osd-operators", // services/osd-operators/{component}
		"backplane",     // services/backplane/{component}
		"configuration-anomaly-detection",
	}

	for _, parent := range parentDirs {
		var candidatePath string
		if parent == "" {
			candidatePath = filepath.Join(gitDirectory, "data", "services", componentName, "app.yml")
		} else {
			candidatePath = filepath.Join(gitDirectory, "data", "services", parent, componentName, "app.yml")
		}

		if _, err := os.Stat(candidatePath); err == nil {
			return candidatePath, nil
		}
	}

	return "", fmt.Errorf("app.yml not found for component %s in any known location", componentName)
}
