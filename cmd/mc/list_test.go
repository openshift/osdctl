package mc

import (
	"testing"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/stretchr/testify/assert"
)

func TestGetClusterNameFromServerURL(t *testing.T) {
	tests := []struct {
		name         string
		serverURL    string
		expectedName string
		expectError  bool
	}{
		{
			name:         "valid server URL with cluster name",
			serverURL:    "https://api.hs-sc-cluster1.example.com",
			expectedName: "hs-sc-cluster1",
			expectError:  false,
		},
		{
			name:         "valid server URL with different cluster name",
			serverURL:    "https://api.test-cluster.domain.com",
			expectedName: "test-cluster",
			expectError:  false,
		},
		{
			name:         "server URL with port",
			serverURL:    "https://api.cluster-name.example.com:8080",
			expectedName: "cluster-name",
			expectError:  false,
		},
		{
			name:         "invalid server URL - no dots",
			serverURL:    "https://invalidurl",
			expectedName: "",
			expectError:  true,
		},
		{
			name:         "invalid server URL - only one part",
			serverURL:    "api",
			expectedName: "",
			expectError:  true,
		},
		{
			name:         "empty server URL",
			serverURL:    "",
			expectedName: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getClusterNameFromServerURL(tt.serverURL)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, "", result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedName, result)
			}
		})
	}
}

func TestProcessServiceClusters(t *testing.T) {
	tests := []struct {
		name          string
		inputShards   []*cmv1.ProvisionShard
		expectedCount int
	}{
		{
			name:          "handles empty input",
			inputShards:   []*cmv1.ProvisionShard{},
			expectedCount: 0,
		},
		{
			name: "handles shards without hypershift config",
			inputShards: []*cmv1.ProvisionShard{
				createProvisionShardWithoutHypershift("shard1"),
			},
			expectedCount: 0,
		},
		{
			name: "handles shards with hypershift config",
			inputShards: []*cmv1.ProvisionShard{
				createProvisionShardWithHypershift("shard1", "https://api.hs-sc-aaabbb.mydomain.com"),
				createProvisionShardWithHypershift("shard2", "https://api.hs-mc-aaabbb.mydomain.com"),
			},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processServiceClusters(tt.inputShards)
			assert.Equal(t, tt.expectedCount, len(result))
		})
	}
}

// Test the behavior when processServiceClusters encounters invalid data
func TestProcessServiceClustersEdgeCases(t *testing.T) {
	// Test with nil slice
	result := processServiceClusters(nil)
	assert.Equal(t, 0, len(result))
	assert.NotNil(t, result) // Should return empty map, not nil

	// Test with slice containing nil elements (this would normally not happen but good to test)
	shards := []*cmv1.ProvisionShard{nil}
	result = processServiceClusters(shards)
	assert.Equal(t, 0, len(result))
}

// Helper functions to create test ProvisionShard objects
func createProvisionShardWithoutHypershift(id string) *cmv1.ProvisionShard {
	shard, _ := cmv1.NewProvisionShard().
		ID(id).
		Build()

	return shard
}

func createProvisionShardWithHypershift(id string, serverURL string) *cmv1.ProvisionShard {
	hsConfig := cmv1.NewServerConfig().Server(serverURL)
	shard, _ := cmv1.NewProvisionShard().
		ID(id).
		HypershiftConfig(hsConfig).
		Build()
	return shard
}
