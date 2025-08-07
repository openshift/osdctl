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
func (lv *LinkValidator) checkURL(url string) error {
	resp, err := lv.httpClient.Head(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// Perform link validation
func (lv *LinkValidator) ValidateLinks(message string) error {
	urls := extractURLs(message)

	for _, url := range urls {
		if err := lv.checkURL(url); err != nil {
			return fmt.Errorf("dead link: %s (%v)", url, err)
		}
	}
	return nil
}
