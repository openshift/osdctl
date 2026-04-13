package requester

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/openshift/osdctl/cmd/vault"

	log "github.com/sirupsen/logrus"
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
		err = json.Unmarshal([]byte(body), &respErr)
		if err != nil {
			return "", err
		}

		return "", fmt.Errorf("request failed: %v %s", resp.Status, respErr)
	}

	return string(body), nil
}

// GetScopedAccessToken gets an access token using the vault path in the configuration key specified
// It will request any scopes listed in the scopes string
func GetScopedAccessToken(authUrl, vaultConfigKey string, scopes string) (string, error) {
	clientId, clientSecret, err := vault.GetCredsFromVault(vaultConfigKey)
	if err != nil {
		return "", err
	}

	reqData := url.Values{
		"grant_type":    {"client_credentials"},
		"scope":         {scopes},
		"client_id":     {clientId},
		"client_secret": {clientSecret},
	}.Encode()

	requester := Requester{
		Method: http.MethodPost,
		Url:    authUrl,
		Data:   string(reqData),
		Headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		},
		SuccessCode: http.StatusOK,
	}

	resp, err := requester.Send()
	if err != nil {
		return "", err
	}

	var respObj map[string]interface{}
	err = json.Unmarshal([]byte(resp), &respObj)
	if err != nil {
		return "", err
	}

	token, ok := respObj["access_token"].(string)
	if !ok {
		return "", fmt.Errorf("access token not present in response")
	}

	log.Infoln("Successfully authenticated")

	return token, nil
}
