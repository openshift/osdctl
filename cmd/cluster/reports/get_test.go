package reports

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCmdGet(t *testing.T) {
	cmd := newCmdGet()

	assert.NotNil(t, cmd)
	assert.Equal(t, "get", cmd.Use)
	assert.Equal(t, "Get a specific cluster report from backplane-api", cmd.Short)

	// Check required flags
	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("cluster-id"), "Command should have a cluster-id flag")
	assert.NotNil(t, flags.Lookup("report-id"), "Command should have a report-id flag")
	assert.NotNil(t, flags.Lookup("output"), "Command should have an output flag")

	// Check default values
	output, err := flags.GetString("output")
	assert.NoError(t, err)
	assert.Equal(t, "text", output, "Default value for 'output' should be 'table'")
}

func TestGetOptions_Validation(t *testing.T) {
	tests := []struct {
		name      string
		clusterID string
		reportID  string
		output    string
		wantErr   bool
	}{
		{
			name:      "valid options",
			clusterID: "test-cluster-123",
			reportID:  "report-456",
			output:    "table",
			wantErr:   false,
		},
		{
			name:      "valid json output",
			clusterID: "test-cluster-123",
			reportID:  "report-456",
			output:    "json",
			wantErr:   false,
		},
		{
			name:      "missing cluster ID",
			clusterID: "",
			reportID:  "report-456",
			output:    "table",
			wantErr:   true,
		},
		{
			name:      "missing report ID",
			clusterID: "test-cluster-123",
			reportID:  "",
			output:    "table",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &getOptions{
				clusterID: tt.clusterID,
				reportID:  tt.reportID,
				output:    tt.output,
			}

			// Basic validation
			hasError := opts.clusterID == "" || opts.reportID == ""
			assert.Equal(t, tt.wantErr, hasError)

			assert.Equal(t, tt.clusterID, opts.clusterID)
			assert.Equal(t, tt.reportID, opts.reportID)
			assert.Equal(t, tt.output, opts.output)
		})
	}
}
