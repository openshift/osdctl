package linkValidator

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLinkValidator_extractURLs(t *testing.T) {

	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single HTTP URL",
			input:    "Please visit http://example.com for more info",
			expected: []string{"http://example.com"},
		},
		{
			name:     "single HTTPS URL",
			input:    "Check out https://docs.openshift.com/rosa/install for details",
			expected: []string{"https://docs.openshift.com/rosa/install"},
		},
		{
			name:     "multiple URLs",
			input:    "Visit https://example.com and http://test.com for more info",
			expected: []string{"https://example.com", "http://test.com"},
		},
		{
			name:     "URL with trailing punctuation",
			input:    "See https://example.com.",
			expected: []string{"https://example.com"},
		},
		{
			name:     "no URLs",
			input:    "This is just text without any URLs",
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractURLs(tc.input)
			if len(result) != len(tc.expected) {
				t.Errorf("Expected %d URLs, got %d: %v", len(tc.expected), len(result), result)
				return
			}

			for i, url := range result {
				if url != tc.expected[i] {
					t.Errorf("Expected URL %s, got %s", tc.expected[i], url)
				}
			}
		})
	}
}

func TestLinkValidator_checkURL(t *testing.T) {
	// Create a test server that returns 200 OK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create a test server that returns 404
	server404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server404.Close()

	// Create a test server that returns 500
	server500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server500.Close()

	lv := NewLinkValidator()

	testCases := []struct {
		name           string
		url            string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "valid URL",
			url:            server.URL,
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name:           "404 URL",
			url:            server404.URL,
			expectedStatus: 404,
			expectError:    false,
		},
		{
			name:           "500 URL",
			url:            server500.URL,
			expectedStatus: 500,
			expectError:    false,
		},
		{
			name:           "invalid URL",
			url:            "not-a-valid-url",
			expectedStatus: 0,
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, err := lv.checkURL(tc.url)
			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if !tc.expectError && statusCode != tc.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tc.expectedStatus, statusCode)
			}
		})
	}
}

func TestLinkValidator_ValidateLinks(t *testing.T) {
	// Create a test server for valid URLs (200 OK)
	serverOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer serverOK.Close()

	// Create a test server for 404 errors (dead links)
	server404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server404.Close()

	// Create a test server for 410 errors (gone links)
	server410 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer server410.Close()

	// Create a test server for warning status (500 internal server error)
	server500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server500.Close()

	lv := NewLinkValidator()

	testCases := []struct {
		name             string
		message          string
		expectError      bool
		expectedWarnings int
	}{
		{
			name:             "message with valid URL",
			message:          "Please check " + serverOK.URL + " for more information",
			expectError:      false,
			expectedWarnings: 0,
		},
		{
			name:             "message with no URLs",
			message:          "This is just a plain text message",
			expectError:      false,
			expectedWarnings: 0,
		},
		{
			name:             "message with 404 URL (dead link error)",
			message:          "Check " + server404.URL + " for details",
			expectError:      true,
			expectedWarnings: 0,
		},
		{
			name:             "message with 410 URL (gone link error)",
			message:          "Visit " + server410.URL + " for info",
			expectError:      true,
			expectedWarnings: 0,
		},
		{
			name:             "message with 500 URL (warning)",
			message:          "See " + server500.URL + " for more",
			expectError:      false,
			expectedWarnings: 1,
		},
		{
			name:             "message with mixed URLs",
			message:          "Good link: " + serverOK.URL + " and warning link: " + server500.URL,
			expectError:      false,
			expectedWarnings: 1,
		},
		{
			name:             "message with network error URL",
			message:          "Check http://this-domain-should-not-exist-12345.com",
			expectError:      true,
			expectedWarnings: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			warnings, err := lv.ValidateLinks(tc.message)
			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if !tc.expectError && len(warnings) != tc.expectedWarnings {
				t.Errorf("Expected %d warnings, got %d: %v", tc.expectedWarnings, len(warnings), warnings)
			}

			// Verify warning structure if warnings are expected
			if !tc.expectError && tc.expectedWarnings > 0 {
				for _, warning := range warnings {
					if warning.URL == "" {
						t.Error("Warning should have a URL")
					}
					if warning.Warning == nil {
						t.Error("Warning should have an error message")
					}
				}
			}
		})
	}
}

func TestValidationResult_Structure(t *testing.T) {
	// Create a test server that returns 403 Forbidden
	server403 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server403.Close()

	lv := NewLinkValidator()

	// Test a scenario that should produce warnings
	message := "Check this link: " + server403.URL
	warnings, err := lv.ValidateLinks(message)

	if err != nil {
		t.Fatalf("Expected no error for 403 status, got: %v", err)
	}

	if len(warnings) != 1 {
		t.Fatalf("Expected 1 warning, got %d", len(warnings))
	}

	warning := warnings[0]

	// Test the ValidationResult structure
	if warning.URL != server403.URL {
		t.Errorf("Expected URL %s, got %s", server403.URL, warning.URL)
	}

	if warning.Warning == nil {
		t.Error("Expected warning to have an error message")
	}

	expectedErrorMsg := "HTTP 403"
	if warning.Warning.Error() != expectedErrorMsg {
		t.Errorf("Expected warning message '%s', got '%s'", expectedErrorMsg, warning.Warning.Error())
	}
}
