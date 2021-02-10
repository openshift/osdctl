package utils

import (
	"fmt"
	"io/ioutil"
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
	return fmt.Errorf("timeout or uknown HTTP error, while trying to access %q", url.String())
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
	resp, err := http.Get(webpage)
	defer func() {
		err = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusOK {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return body, err
		}
		body = bodyBytes
	}
	return body, err
}
