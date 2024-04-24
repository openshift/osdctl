package dynatrace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/spf13/viper"
)

const (
	authURL     string = "https://sso.dynatrace.com/sso/oauth2/token"
	DTVaultPath string = "dt_vault_path"
	VaultAddr   string = "vault_address"
)

type Requester struct {
	method      string
	url         string
	data        string
	headers     map[string]string
	successCode int
}

func (rh *Requester) send() (string, error) {
	client := http.Client{
		Timeout: time.Second * 10,
	}

	var req *http.Request
	var err error
	if rh.data != "" {
		req, err = http.NewRequest(rh.method, rh.url, bytes.NewBuffer([]byte(rh.data)))
	} else {
		req, err = http.NewRequest(rh.method, rh.url, nil)
	}

	if err != nil {
		return "", fmt.Errorf("failed to build request %v", err)
	}

	for hdr, val := range rh.headers {
		req.Header.Set(hdr, val)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode != rh.successCode {
		return "", fmt.Errorf("request failed: %v", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func getVaultPath() (addr, path string, error error) {
	if !viper.IsSet(VaultAddr) {
		return "", "", fmt.Errorf("key %s is not set in config file", VaultAddr)
	}
	vaultAddr := viper.GetString(VaultAddr)

	if !viper.IsSet(DTVaultPath) {
		return "", "", fmt.Errorf("key %s is not set in config file", DTVaultPath)
	}
	vaultPath := viper.GetString(DTVaultPath)

	return vaultAddr, vaultPath, nil
}

func getAccessToken() (string, error) {
	vaultAddr, vaultPath, err := getVaultPath()
	if err != nil {
		return "", err
	}

	err = setupVaultToken(vaultAddr, vaultPath)
	if err != nil {
		return "", err
	}

	clientID, clientSecret, err := getSecretFromVault(vaultAddr, vaultPath)
	if err != nil {
		return "", err
	}

	reqData := url.Values{
		"grant_type":    {"client_credentials"},
		"scope":         {"storage:logs:read storage:buckets:read"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}.Encode()

	requester := Requester{
		method: http.MethodPost,
		url:    authURL,
		data:   string(reqData),
		headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		},
		successCode: http.StatusOK,
	}

	resp, err := requester.send()
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

	fmt.Println("Successfully authenticated with DynaTrace")

	return token, nil
}

type DTQueryPayload struct {
	Query string `json:"query"`
}

type DTPollResult struct {
	State    string `json:"state"`
	Progress int    `json:"progress"`
	Result   Result `json:"result"`
}

type Result struct {
	Records []LogContent `json:"records"`
}

type LogContent struct {
	Content string `json:"content"`
}

type ExecuteResponse struct {
	RequestToken string `json:"requestToken"`
}

func getRequestToken(query string, dtURL string, accessToken string) (requestToken string, error error) {
	payload := DTQueryPayload{
		Query: query,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	requester := Requester{
		method: http.MethodPost,
		url:    dtURL + "platform/storage/query/v1/query:execute",
		data:   string(payloadJSON),
		headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + accessToken,
		},
		successCode: http.StatusAccepted,
	}

	resp, err := requester.send()
	if err != nil {
		return "", err
	}

	var execResp ExecuteResponse
	err = json.Unmarshal([]byte(resp), &execResp)
	if err != nil {
		return "", err
	}

	return execResp.RequestToken, nil
}

func getLogs(dtURL string, accessToken string, requestToken string, dumpWriter io.Writer) error {
	reqData := url.Values{
		"request-token": {requestToken},
	}.Encode()

	requester := Requester{
		method: http.MethodGet,
		url:    dtURL + "platform/storage/query/v1/query:poll?" + reqData,
		headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + accessToken,
		},
		successCode: http.StatusOK,
	}

	resp, err := requester.send()
	if err != nil {
		return err
	}

	var dtPollRes DTPollResult
	err = json.Unmarshal([]byte(resp), &dtPollRes)
	if err != nil {
		return err
	}

	for _, result := range dtPollRes.Result.Records {
		content := result.Content
		if dumpWriter != nil {
			dumpWriter.Write([]byte(fmt.Sprintf("%s\n", content)))
		} else {
			fmt.Println(content)
		}
	}

	return nil
}
