package resize

import (
	"testing"
)

func TestExtractInstanceClass_AWS(t *testing.T) {
	tests := []struct {
		name         string
		instanceType string
		expected     string
	}{
		{
			name:         "AWS m5 instance",
			instanceType: "m5.4xlarge",
			expected:     "m5",
		},
		{
			name:         "AWS m6i instance",
			instanceType: "m6i.8xlarge",
			expected:     "m6i",
		},
		{
			name:         "AWS m5 small instance",
			instanceType: "m5.2xlarge",
			expected:     "m5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractInstanceClass(tt.instanceType)
			if err != nil {
				t.Error(err)
			}
			if result != tt.expected {
				t.Errorf("extractInstanceClass(%s) = %s, expected %s", tt.instanceType, result, tt.expected)
			}
		})
	}
}

func TestInstanceClassValidation_AWS(t *testing.T) {
	tests := []struct {
		name            string
		currentInstance string
		newInstance     string
		shouldFail      bool
	}{
		{
			name:            "Same class AWS m5",
			currentInstance: "m5.2xlarge",
			newInstance:     "m5.4xlarge",
			shouldFail:      false,
		},
		{
			name:            "Different class AWS m5 to m6i",
			currentInstance: "m5.4xlarge",
			newInstance:     "m6i.8xlarge",
			shouldFail:      true,
		},
		{
			name:            "Different class AWS m6i to m5",
			currentInstance: "m6i.8xlarge",
			newInstance:     "m5.4xlarge",
			shouldFail:      true,
		},
		{
			name:            "Same class AWS m6i",
			currentInstance: "m6i.4xlarge",
			newInstance:     "m6i.8xlarge",
			shouldFail:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentClass, err := extractInstanceClass(tt.currentInstance)
			if err != nil && !tt.shouldFail {
				t.Error(err)
			}
			newClass, err := extractInstanceClass(tt.newInstance)
			if err != nil && !tt.shouldFail {
				t.Error(err)
			}
			failed := currentClass != newClass

			if failed != tt.shouldFail {
				t.Errorf("Instance class validation for %s -> %s: expected shouldFail=%v, got %v (currentClass=%s, newClass=%s)",
					tt.currentInstance, tt.newInstance, tt.shouldFail, failed, currentClass, newClass)
			}
		})
	}
}
