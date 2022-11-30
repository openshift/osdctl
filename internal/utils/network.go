package utils

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// IsOnline checks the provided URL for connectivity
func IsOnline(url url.URL) error {
	timeout := 2 * time.Second
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(url.String())

	if err != nil {
		return fmt.Errorf("%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// Non-200 http statuses are considered error
	return fmt.Errorf("timeout or unknown HTTP error, while trying to access %q", url.String())
}

// IsValidUrl tests a string to determine if it is a well-structured url or not.
func IsValidUrl(toTest string) bool {
	_, err := url.ParseRequestURI(toTest)
	if err != nil {
		return false
	}

	u, err := url.Parse(toTest)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	return true
}

func CurlThis(webpage string) (body []byte, err error) {
	// For the following line we have to disable the gosec linter, otherwise G107 will get thrown
	// G107 is about handling non const URLs. We are reading a URL from a file. This can be malicious.
	resp, err := http.Get(webpage) //#nosec G107 -- url cannot be constant
	defer func() {
		err = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return body, err
		}
		body = bodyBytes
	}
	return body, err
}
