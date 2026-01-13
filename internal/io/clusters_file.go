package io

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// ClustersFile represents the structure of a cluster file for mass cluster operations
type ClustersFile struct {
	Clusters []string `json:"clusters"`
}

// Regular expression for valid cluster IDs - alphanumeric characters and hyphens only
var validClusterIDRegex = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// ParseAndValidateClustersFile reads, parses, and validates a cluster file
// Returns a slice of validated cluster IDs for mass cluster operations
func ParseAndValidateClustersFile(filePath string) ([]string, error) {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read clusters file: %w", err)
	}

	var clustersFile ClustersFile
	if err := json.Unmarshal(file, &clustersFile); err != nil {
		return nil, fmt.Errorf("failed to parse clusters file: %w", err)
	}

	// Validate each cluster ID using regex
	for i, id := range clustersFile.Clusters {
		if !validClusterIDRegex.MatchString(id) {
			return nil, fmt.Errorf("clusters file contains invalid cluster ID at index %d: '%s' - only alphanumeric characters and hyphens are allowed", i, id)
		}
	}

	return clustersFile.Clusters, nil
}
