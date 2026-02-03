package cluster

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// Test buildTemplateParameters - tests creating template parameter array
func Test_buildTemplateParameters(t *testing.T) {
	tests := []struct {
		name     string
		failures []string
		expected []string
	}{
		{
			name:     "single failure returns parameter",
			failures: []string{"cloud.openshift.com"},
			expected: []string{"FAILURE_LIST=cloud.openshift.com"},
		},
		{
			name:     "multiple failures returns parameter list",
			failures: []string{"cloud.openshift.com", "quay.io"},
			expected: []string{"FAILURE_LIST=cloud.openshift.com, quay.io"},
		},
		{
			name:     "three failures returns parameter list",
			failures: []string{"cloud.openshift.com", "quay.io", "registry.redhat.io"},
			expected: []string{"FAILURE_LIST=cloud.openshift.com, quay.io, registry.redhat.io"},
		},
		{
			name:     "empty failures returns empty parameter",
			failures: []string{},
			expected: []string{"FAILURE_LIST="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTemplateParameters(tt.failures)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("buildTemplateParameters() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// Test formatFailureDisplay - tests formatting failure output
func Test_formatFailureDisplay(t *testing.T) {
	tests := []struct {
		name     string
		category string
		failures []string
		contains []string // Strings that should be in output
	}{
		{
			name:     "single failure formatting",
			category: "Pull Secret Issues",
			failures: []string{"cloud.openshift.com"},
			contains: []string{
				"Pull Secret Issues",
				"Found 1 failure(s)",
				"1. cloud.openshift.com",
			},
		},
		{
			name:     "multiple failures formatting",
			category: "Pull Secret Issues",
			failures: []string{"registry.redhat.io", "quay.io"},
			contains: []string{
				"Pull Secret Issues",
				"Found 2 failure(s)",
				"1. registry.redhat.io",
				"2. quay.io",
			},
		},
		{
			name:     "three failures formatting",
			category: "Pull Secret Issues",
			failures: []string{"cloud.openshift.com", "quay.io", "registry.redhat.io"},
			contains: []string{
				"Pull Secret Issues",
				"Found 3 failure(s)",
				"1. cloud.openshift.com",
				"2. quay.io",
				"3. registry.redhat.io",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatFailureDisplay(tt.category, tt.failures)
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("formatFailureDisplay() missing expected string %q\nGot: %v", expected, result)
				}
			}
		})
	}
}

// Test recordServiceLogFailure - tests failure recording with skipServiceLogs flag
func Test_recordServiceLogFailure(t *testing.T) {
	tests := []struct {
		name            string
		skipServiceLogs bool
		recordings      []struct {
			template   string
			authSource string
		}
		expectedCounts map[string]int
	}{
		{
			name:            "records failure when not skipping",
			skipServiceLogs: false,
			recordings: []struct {
				template   string
				authSource string
			}{
				{"template1.json", "cloud.openshift.com"},
			},
			expectedCounts: map[string]int{"template1.json": 1},
		},
		{
			name:            "skips recording when flag set",
			skipServiceLogs: true,
			recordings: []struct {
				template   string
				authSource string
			}{
				{"template1.json", "cloud.openshift.com"},
			},
			expectedCounts: map[string]int{},
		},
		{
			name:            "records multiple failures for same template",
			skipServiceLogs: false,
			recordings: []struct {
				template   string
				authSource string
			}{
				{"template1.json", "cloud.openshift.com"},
				{"template1.json", "quay.io"},
			},
			expectedCounts: map[string]int{"template1.json": 2},
		},
		{
			name:            "records failures for different templates",
			skipServiceLogs: false,
			recordings: []struct {
				template   string
				authSource string
			}{
				{"template1.json", "cloud.openshift.com"},
				{"template2.json", "quay.io"},
			},
			expectedCounts: map[string]int{"template1.json": 1, "template2.json": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &validatePullSecretExtOptions{
				skipServiceLogs:      tt.skipServiceLogs,
				failuresByServiceLog: make(map[string][]string),
				log:                  logrus.New(),
			}

			// Record all failures
			for _, rec := range tt.recordings {
				opts.recordServiceLogFailure(rec.template, rec.authSource)
			}

			// Verify counts for each template
			for template, expectedCount := range tt.expectedCounts {
				actualCount := len(opts.failuresByServiceLog[template])
				if actualCount != expectedCount {
					t.Errorf("recordServiceLogFailure() recorded %d failures for %s, expected %d",
						actualCount, template, expectedCount)
				}
			}

			// Verify no unexpected templates were recorded
			if len(opts.failuresByServiceLog) != len(tt.expectedCounts) {
				t.Errorf("recordServiceLogFailure() recorded %d templates, expected %d",
					len(opts.failuresByServiceLog), len(tt.expectedCounts))
			}
		})
	}
}

func Test_getPullSecretAuthEmail(t *testing.T) {
	tests := []struct {
		name          string
		secret        *corev1.Secret
		expectedEmail string
		expectedError error
	}{
		{
			name:          "Missing dockerconfigjson",
			secret:        &corev1.Secret{Data: map[string][]byte{}},
			expectedError: ErrSecretMissingDockerConfigJson,
		},
		{
			name:          "Missing cloud.openshift.com auth",
			secret:        &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{}}")}},
			expectedError: &ErrorSecretAuthNotFound{auth: "cloud.openshift.com"},
		},
		{
			name:          "Missing email",
			secret:        &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{\"cloud.openshift.com\":{}}}")}},
			expectedError: &ErrorAuthEmailNotFound{auth: "cloud.openshift.com"},
		},
		{
			name:          "Valid pull secret",
			secret:        &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{\"cloud.openshift.com\":{\"email\":\"foo@bar.com\"}}}")}},
			expectedEmail: "foo@bar.com",
			expectedError: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, err := getPullSecretAuthEmail(tt.secret, "cloud.openshift.com")
			if email != tt.expectedEmail {
				t.Errorf("getPullSecretEmail() email = %v, expectedEmail %v", email, tt.expectedEmail)
			}
			if !reflect.DeepEqual(err, tt.expectedError) {
				t.Errorf("getPullSecretEmail() err = %v, expectedEmail %v", err, tt.expectedError)
			}
			//if err != nil {
			//	fmt.Fprintf(os.Stderr, "Got error type:'%T' vs Expected:'%T\n", err, tt.expectedError)
			//}
		})
	}
}
