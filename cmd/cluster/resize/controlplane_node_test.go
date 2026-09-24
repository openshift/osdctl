package resize

import (
	"strings"
	"testing"
)

func TestValidateAWSInstanceTypeChange(t *testing.T) {
	tests := []struct {
		name                string
		currentInstanceType string
		newInstanceType     string
		errContains         string
	}{
		{
			name:                "same general purpose family",
			currentInstanceType: "m5.2xlarge",
			newInstanceType:     "m5.4xlarge",
		},
		{
			name:                "m5 to m6i",
			currentInstanceType: "m5.4xlarge",
			newInstanceType:     "m6i.4xlarge",
		},
		{
			name:                "unsupported requested instance type",
			currentInstanceType: "m5.4xlarge",
			newInstanceType:     "m6i.not-a-size",
			errContains:         "instance type m6i.not-a-size not supported for controlplane nodes",
		},
		{
			name:                "m6i to m5 is not allowed",
			currentInstanceType: "m6i.4xlarge",
			newInstanceType:     "m5.4xlarge",
			errContains:         "cannot change instance family from m6i to m5",
		},
		{
			name:                "other family change is not allowed",
			currentInstanceType: "m5.4xlarge",
			newInstanceType:     "r5.4xlarge",
			errContains:         "cannot change instance family from m5 to r5",
		},
		{
			name:                "invalid current instance type",
			currentInstanceType: "m5",
			newInstanceType:     "m6i.4xlarge",
			errContains:         "instance type m5 is not a valid instance type",
		},
		{
			name:                "missing instance size",
			currentInstanceType: "m5.",
			newInstanceType:     "m6i.4xlarge",
			errContains:         "instance type m5. is not a valid instance type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAWSInstanceTypeChange(tt.currentInstanceType, tt.newInstanceType)
			if tt.errContains == "" {
				if err != nil {
					t.Fatalf("validateAWSInstanceTypeChange() error = %v", err)
				}
				return
			}

			if err == nil || !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("validateAWSInstanceTypeChange() error = %v, want error containing %q", err, tt.errContains)
			}
		})
	}
}
