package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type responseError struct {
	Records json.RawMessage `json:"error"`
}

type Requester struct {
	Method      string
	Url         string
	Data        string
	Headers     map[string]string
	SuccessCode int
}

func (rh *Requester) Send() (string, error) {
	client := http.Client{
		Timeout: time.Second * 600,
	}

	var req *http.Request
	var err error
	if rh.Data != "" {
		req, err = http.NewRequest(rh.Method, rh.Url, bytes.NewBuffer([]byte(rh.Data)))
	} else {
		req, err = http.NewRequest(rh.Method, rh.Url, nil)
	}

	if err != nil {
		return "", fmt.Errorf("failed to build request %v", err)
	}

	for hdr, val := range rh.Headers {
		req.Header.Set(hdr, val)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != rh.SuccessCode {
		var respErr responseError
		err = json.Unmarshal(body, &respErr)
		if err != nil || len(respErr.Records) == 0 {
			return "", fmt.Errorf("request failed: %s: %s", resp.Status, body)
		}

		return "", fmt.Errorf("request failed: %s %s", resp.Status, respErr.Records)
	}

	return string(body), nil
}
