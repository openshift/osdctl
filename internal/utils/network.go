package utils

import (
	"fmt"
	"io"
	"net"
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

// IsProxyReachable checks the providede URL for connectivity
func IsProxyReachable(url url.URL) error {
	timeout := 2 * time.Second
	resp, err := net.DialTimeout("tcp", url.Hostname()+":"+url.Port(), timeout)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Close(); err != nil {
			fmt.Printf("error closing connection: %s", err.Error())
		}
	}()
	return nil
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

// CurlThisAuthorized will curl the given webpage with a Authorization header
func CurlThisAuthorized(webpage string, token string, proxy string) ([]byte, error) {
	// For the following line we have to disable the gosec linter, otherwise G107 will get thrown
	// G107 is about handling non const URLs. We are reading a URL from a file. This can be malicious.
	client := http.Client{}

	if proxy != "" {
		purl, err := url.Parse(proxy)
		if err != nil {
			return nil, err
		}
		err = IsProxyReachable(*purl)
		if err != nil {
			return nil, fmt.Errorf("unable to connect to proxy, are you connected to the VPN?\n%w", err)
		}
		client = http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(purl),
			},
		}
	}

	req, err := http.NewRequest("GET", webpage, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return bodyBytes, nil
	}
	// Non-200 http status is considered error
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("StatusCode: %d, HTTP error, while trying to access %q", resp.StatusCode, webpage)
	}
	return nil, fmt.Errorf("StatusCode: %d, HTTP error, while trying to access %q: %s", resp.StatusCode, webpage, string(bodyBytes))
}
