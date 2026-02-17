package cad

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCADClusterConfig(t *testing.T) {
	tests := []struct {
		name              string
		environment       string
		expectedClusterID string
		expectedNamespace string
	}{
		{
			name:              "stage environment",
			environment:       "stage",
			expectedClusterID: cadClusterIDStage,
			expectedNamespace: "configuration-anomaly-detection-stage",
		},
		{
			name:              "production environment",
			environment:       "production",
			expectedClusterID: cadClusterIDProd,
			expectedNamespace: "configuration-anomaly-detection-production",
		},
		{
			name:              "empty environment defaults to production",
			environment:       "",
			expectedClusterID: cadClusterIDProd,
			expectedNamespace: "configuration-anomaly-detection-production",
		},
		{
			name:              "unknown environment defaults to production",
			environment:       "unknown",
			expectedClusterID: cadClusterIDProd,
			expectedNamespace: "configuration-anomaly-detection-production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &cadRunOptions{environment: tt.environment}
			clusterID, namespace := opts.getCADClusterConfig()

			assert.Equal(t, tt.expectedClusterID, clusterID, "cluster ID should match")
			assert.Equal(t, tt.expectedNamespace, namespace, "namespace should match")
		})
	}
}

func TestPipelineRunTemplate(t *testing.T) {
	tests := []struct {
		name              string
		clusterID         string
		investigation     string
		cadNamespace      string
		isDryRun          bool
		expectedNamespace string
	}{
		{
			name:              "basic pipeline run",
			clusterID:         "test-cluster-123",
			investigation:     "chgm",
			cadNamespace:      "configuration-anomaly-detection-production",
			isDryRun:          false,
			expectedNamespace: "configuration-anomaly-detection-production",
		},
		{
			name:              "stage environment pipeline run",
			clusterID:         "stage-cluster-456",
			investigation:     "cmbb",
			cadNamespace:      "configuration-anomaly-detection-stage",
			isDryRun:          false,
			expectedNamespace: "configuration-anomaly-detection-stage",
		},
		{
			name:              "dry-run pipeline run",
			clusterID:         "test-cluster-789",
			investigation:     "ai",
			cadNamespace:      "configuration-anomaly-detection-production",
			isDryRun:          true,
			expectedNamespace: "configuration-anomaly-detection-production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &cadRunOptions{
				clusterID:     tt.clusterID,
				investigation: tt.investigation,
				isDryRun:      tt.isDryRun,
			}

			result := opts.pipelineRunTemplate(tt.cadNamespace)

			gvk := result.GroupVersionKind()
			assert.Equal(t, "tekton.dev", gvk.Group, "group should be tekton.dev")
			assert.Equal(t, "v1beta1", gvk.Version, "version should be v1beta1")
			assert.Equal(t, "PipelineRun", gvk.Kind, "kind should be PipelineRun")

			metadata := result.Object["metadata"].(map[string]interface{})
			assert.Equal(t, tt.expectedNamespace, metadata["namespace"], "namespace should match")

			spec := result.Object["spec"].(map[string]interface{})

			params := spec["params"].([]map[string]interface{})
			assert.Len(t, params, 3, "should have 3 params")

			assert.Equal(t, "cluster-id", params[0]["name"], "first param should be cluster-id")
			assert.Equal(t, tt.clusterID, params[0]["value"], "cluster-id value should match")

			assert.Equal(t, "investigation", params[1]["name"], "second param should be investigation")
			assert.Equal(t, tt.investigation, params[1]["value"], "investigation value should match")

			assert.Equal(t, "dry-run", params[2]["name"], "third param should be dry-run")
			assert.Equal(t, tt.isDryRun, params[2]["value"], "dry-run value should match")
		})
	}
}
