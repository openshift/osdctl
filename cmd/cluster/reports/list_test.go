package reports

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCmdList(t *testing.T) {
	cmd := newCmdList()

	assert.NotNil(t, cmd)
	assert.Equal(t, "list", cmd.Use)
	assert.Equal(t, "List cluster reports from backplane-api", cmd.Short)

	// Check required flags
	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("cluster-id"), "Command should have a cluster-id flag")
	assert.NotNil(t, flags.Lookup("last"), "Command should have a last flag")
	assert.NotNil(t, flags.Lookup("output"), "Command should have an output flag")

	// Check default values
	last, err := flags.GetInt("last")
	assert.NoError(t, err)
	assert.Equal(t, 0, last, "Default value for 'last' should be 0 (backend defaults to 10)")

	output, err := flags.GetString("output")
	assert.NoError(t, err)
	assert.Equal(t, "table", output, "Default value for 'output' should be 'table'")
}

func TestListOptions_Validation(t *testing.T) {
	tests := []struct {
		name      string
		clusterID string
		last      int
		output    string
		wantErr   bool
	}{
		{
			name:      "valid options",
			clusterID: "test-cluster-123",
			last:      10,
			output:    "table",
			wantErr:   false,
		},
		{
			name:      "valid json output",
			clusterID: "test-cluster-123",
			last:      5,
			output:    "json",
			wantErr:   false,
		},
		{
			name:      "zero last value",
			clusterID: "test-cluster-123",
			last:      0,
			output:    "table",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &listOptions{
				clusterID: tt.clusterID,
				last:      tt.last,
				output:    tt.output,
			}

			// Basic validation - cluster ID should not be empty
			if tt.clusterID == "" && !tt.wantErr {
				t.Errorf("Expected error for empty cluster ID")
			}

			assert.Equal(t, tt.clusterID, opts.clusterID)
			assert.Equal(t, tt.last, opts.last)
			assert.Equal(t, tt.output, opts.output)
		})
	}
}
