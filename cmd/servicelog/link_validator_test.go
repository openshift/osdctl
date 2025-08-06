package servicelog

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLinkValidator_extractURLs(t *testing.T) {
	lv := NewLinkValidator(false)

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
			result := lv.extractURLs(tc.input)
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

	lv := NewLinkValidator()

	testCases := []struct {
		name        string
		url         string
		expectError bool
	}{
		{
			name:        "valid URL",
			url:         server.URL,
			expectError: false,
		},
		{
			name:        "404 URL",
			url:         server404.URL,
			expectError: true,
		},
		{
			name:        "invalid URL",
			url:         "not-a-valid-url",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := lv.checkURL(tc.url)
			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestLinkValidator_validateLinks(t *testing.T) {
	// Create a test server for valid URLs
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	lv := NewLinkValidator()

	testCases := []struct {
		name        string
		message     string
		expectError bool
	}{
		{
			name:        "message with valid URL",
			message:     "Please check " + server.URL + " for more information",
			expectError: false,
		},
		{
			name:        "message with no URLs",
			message:     "This is just a plain text message",
			expectError: false,
		},
		{
			name:        "message with invalid URL",
			message:     "Check http://this-domain-should-not-exist-12345.com",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := lv.validateLinks(tc.message)
			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestLinkValidator_SetSkipValidation(t *testing.T) {
	lv := NewLinkValidator()
	lv.skipLinkCheck = true

	// Even with a bad URL, validation should be skipped
	err := lv.validateLinks("Check http://this-domain-should-not-exist-12345.com")
	if err != nil {
		t.Errorf("Expected no error when validation is skipped, but got: %v", err)
	}
}
