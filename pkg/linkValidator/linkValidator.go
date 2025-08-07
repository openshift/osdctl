package linkValidator

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	Timeout = time.Second * 5
)

// LinkValidator handles validation of URLs in service log messages
type LinkValidator struct {
	timeout    time.Duration
	httpClient *http.Client
}

// ValidationResult holds a URL validation warning for non-fatal HTTP errors
type ValidationResult struct {
	URL     string
	Warning error
}

// NewLinkValidator creates a new LinkValidator with default settings
func NewLinkValidator() *LinkValidator {
	return &LinkValidator{
		timeout:    Timeout,
		httpClient: &http.Client{Timeout: Timeout},
	}
}

// Extract URLs from service log
func extractURLs(text string) []string {
	urlRegex := regexp.MustCompile(`https?://[^\s]+`)
	matches := urlRegex.FindAllString(text, -1)

	var cleanURLs []string
	for _, match := range matches {
		// Remove common trailing punctuation
		cleanURL := strings.TrimRight(match, ".,;:!?)]}")
		cleanURLs = append(cleanURLs, cleanURL)
	}

	return cleanURLs
}

// Check if URL is active
func (lv *LinkValidator) checkURL(url string) (int, error) {
	resp, err := lv.httpClient.Head(url)
	// Check for network errors
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// Perform link validation
func (lv *LinkValidator) ValidateLinks(message string) ([]ValidationResult, error) {
	urls := extractURLs(message)
	var warnings []ValidationResult

	for _, url := range urls {
		statusCode, err := lv.checkURL(url)
		if err != nil {
			return nil, fmt.Errorf("network error for link validation %v", err)
		}
		if statusCode == 404 || statusCode == 410 {
			return nil, fmt.Errorf("dead link: %s (HTTP %d)", url, statusCode)
		}
		if statusCode >= 400 {
			warnings = append(warnings, ValidationResult{
				URL:     url,
				Warning: fmt.Errorf("HTTP %d", statusCode),
			})
		}
	}
	return warnings, nil
}
