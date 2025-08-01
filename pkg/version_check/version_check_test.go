package version_check

import "testing"

func TestTrimVersionString(t *testing.T) {
	tests := []struct {
		expected string
		given    string
	}{
		{
			expected: "1.1.1",
			given:    "1.1.1",
		},
		{
			expected: "1.2.3",
			given:    "v1.2.3",
		},
		{
			expected: "2.3.4",
			given:    "2.3.4-build",
		},
		{
			expected: "3.4.5",
			given:    "v3.4.5-next",
		},
	}

	for _, test := range tests {
		result := trimVersionString(test.given)
		if test.expected != result {
			t.Errorf("Expected %s, given %s, got %s", test.expected, test.given, result)
		}
	}
}

func TestShouldRunVersionCheck(t *testing.T) {
	tests := []struct {
		commandName    string
		checkFlagValue bool
		expected       bool
	}{
		{
			commandName:    "upgrade",
			checkFlagValue: false,
			expected:       false,
		},
		{
			commandName:    "version",
			checkFlagValue: false,
			expected:       false,
		},
		{
			commandName:    "myCommand",
			checkFlagValue: false,
			expected:       true,
		},
		{
			commandName:    "myCommand",
			checkFlagValue: true,
			expected:       false,
		},
	}

	for _, test := range tests {
		if shouldRunVersionCheck(test.checkFlagValue, test.commandName) != test.expected {
			t.Errorf("Skip Version Check Test failed with incorrect result. Values: %+v", test)
		}
	}
}
