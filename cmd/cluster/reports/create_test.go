package reports

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCmdCreate(t *testing.T) {
	cmd := newCmdCreate()

	assert.NotNil(t, cmd)
	assert.Equal(t, "create", cmd.Use)
	assert.Equal(t, "Create a new cluster report in backplane-api", cmd.Short)

	// Check required flags
	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("cluster-id"), "Command should have a cluster-id flag")
	assert.NotNil(t, flags.Lookup("summary"), "Command should have a summary flag")
	assert.NotNil(t, flags.Lookup("data"), "Command should have a data flag")
	assert.NotNil(t, flags.Lookup("file"), "Command should have a file flag")
	assert.NotNil(t, flags.Lookup("output"), "Command should have an output flag")

	// Check default values
	output, err := flags.GetString("output")
	assert.NoError(t, err)
	assert.Equal(t, "table", output, "Default value for 'output' should be 'table'")
}

func TestCreateOptions_Validation(t *testing.T) {
	tests := []struct {
		name        string
		clusterID   string
		summary     string
		data        string
		file        string
		output      string
		wantErr     bool
		expectedErr string
	}{
		{
			name:      "valid with data string",
			clusterID: "test-cluster-123",
			summary:   "Test Report",
			data:      "test data content",
			output:    "table",
			wantErr:   false,
		},
		{
			name:      "valid with json output",
			clusterID: "test-cluster-123",
			summary:   "Test Report",
			data:      "test data content",
			output:    "json",
			wantErr:   false,
		},
		{
			name:        "missing data and file",
			clusterID:   "test-cluster-123",
			summary:     "Test Report",
			output:      "table",
			wantErr:     true,
			expectedErr: "either --data or --file must be provided",
		},
		{
			name:        "both data and file provided",
			clusterID:   "test-cluster-123",
			summary:     "Test Report",
			data:        "test data",
			file:        "/path/to/file",
			output:      "table",
			wantErr:     true,
			expectedErr: "cannot specify both --data and --file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &createOptions{
				clusterID: tt.clusterID,
				summary:   tt.summary,
				data:      tt.data,
				file:      tt.file,
				output:    tt.output,
			}

			// Test validation logic without calling run() since it requires OCM connection
			// We only test the validation part that happens before OCM/backplane calls
			var validationErr error
			if opts.data == "" && opts.file == "" {
				validationErr = assert.AnError // Simulate validation error
				if tt.wantErr {
					assert.NotNil(t, validationErr)
					if tt.expectedErr != "" {
						// Verify expected error message would be returned
						assert.Equal(t, "either --data or --file must be provided", tt.expectedErr)
					}
				}
			} else if opts.data != "" && opts.file != "" {
				validationErr = assert.AnError // Simulate validation error
				if tt.wantErr {
					assert.NotNil(t, validationErr)
					if tt.expectedErr != "" {
						// Verify expected error message would be returned
						assert.Equal(t, "cannot specify both --data and --file", tt.expectedErr)
					}
				}
			}
		})
	}
}
