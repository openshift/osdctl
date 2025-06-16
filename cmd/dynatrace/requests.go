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
	VaultAddr string = "vault_address"

	authURL string = "https://sso.dynatrace.com/sso/oauth2/token"

	// Logs
	DTStorageVaultPath string = "dt_vault_path"
	DTStorageScopes    string = "storage:logs:read storage:events:read storage:buckets:read"

	// Dashboards
	DTDocumentVaultPath string = "dt_document_vault_path"
	DTDocumentScopes    string = "document:documents:read"
	DTDashboardType     string = "dashboard"
)

type DTRequestError struct {
	Records json.RawMessage `json:"error"`
}

type Requester struct {
	method      string
	url         string
	data        string
	headers     map[string]string
	successCode int
}

func (rh *Requester) send() (string, error) {
	client := http.Client{
		Timeout: time.Second * 600,
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != rh.successCode {
		var dtError DTRequestError
		err = json.Unmarshal([]byte(body), &dtError)
		if err != nil {
			return "", err
		}

		return "", fmt.Errorf("request failed: %v %s", resp.Status, dtError)
	}

	return string(body), nil
}

func getVaultPath(vaultPathKey string) (addr, path string, error error) {
	if !viper.IsSet(VaultAddr) {
		return "", "", fmt.Errorf("key '%s' is not set in config file", VaultAddr)
	}
	vaultAddr := viper.GetString(VaultAddr)

	if !viper.IsSet(vaultPathKey) {
		return "", "", fmt.Errorf("key '%s' is not set in config file", vaultPathKey)
	}
	vaultPath := viper.GetString(vaultPathKey)

	return vaultAddr, vaultPath, nil
}

// getScopedAccessToken gets an access token using the vault path in the configuration key specified
// It will request any scopes listed in the scopes string
func getScopedAccessToken(configKey string, scopes string) (string, error) {
	vaultAddr, vaultPath, err := getVaultPath(configKey)
	if err != nil {
		return "", err
	}

	err = setupVaultToken(vaultAddr)
	if err != nil {
		return "", nil
	}

	clientId, clientSecret, err := getSecretFromVault(vaultAddr, vaultPath)
	if err != nil {
		return "", nil
	}

	reqData := url.Values{
		"grant_type":    {"client_credentials"},
		"scope":         {scopes},
		"client_id":     {clientId},
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

func getDocumentAccessToken() (string, error) {
	return getScopedAccessToken(DTDocumentVaultPath, DTDocumentScopes)
}

func getStorageAccessToken() (string, error) {
	return getScopedAccessToken(DTStorageVaultPath, DTStorageScopes)
}

type DTQueryPayload struct {
	Query            string `json:"query"`
	MaxResultRecords int    `json:"maxResultRecords"`
}

type DTPollResult struct {
	State string `json:"state"`
}

type DTLogsPollResult struct {
	State    string    `json:"state"`
	Progress int       `json:"progress"`
	Result   LogResult `json:"result"`
}

type LogResult struct {
	Records []LogContent `json:"records"`
}

type LogContent struct {
	Content string `json:"content"`
}

type DTEventsPollResult struct {
	State    string        `json:"state"`
	Progress int           `json:"progress"`
	Result   DTEventResult `json:"result"`
}

type DTEventResult struct {
	Records []json.RawMessage `json:"records"`
}

type DTExecuteState struct {
	State      string `json:"state"`
	TTLSeconds int    `json:"ttlSeconds"`
}

type DTExecuteToken struct {
	RequestToken string `json:"requestToken"`
}

type DTExecuteResults struct {
	Result []json.RawMessage `json:"records"`
}

type DTDocumentResult struct {
	Documents []DTDocument `json:"documents"`
}

type DTDocument struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

func getDTQueryExecution(dtURL string, accessToken string, query string) (reqToken string, error error) {
	// Note: Currently we are setting a limit of 20,000 lines to pull from Dynatrace
	// due to a limitation in dynatrace to pull all logs. This limitation can be revoked
	// once https://community.dynatrace.com/t5/Product-ideas/Pagination-in-DQL-results/idi-p/248282#M45818
	// is addressed. Then we can implement https://issues.redhat.com/browse/OSD-24349 to get rid of this limitation.
	payload := DTQueryPayload{
		Query:            query,
		MaxResultRecords: 20000,
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

	var resp string
	for {
		resp, err = requester.send()
		if err != nil {
			return "", err
		}
		var execState DTExecuteState
		err = json.Unmarshal([]byte(resp), &execState)
		if err != nil {
			return "", err
		}

		if execState.State != "RUNNING" && execState.State != "SUCCEEDED" {
			return "", fmt.Errorf("query failed")
		}

		break
	}

	var state DTExecuteState
	err = json.Unmarshal([]byte(resp), &state)
	if err != nil {
		return "", err
	}

	if state.State != "RUNNING" && state.State != "SUCCEEDED" {
		return "", fmt.Errorf("query failed")
	}

	// acquire the request token for the execution
	var token DTExecuteToken
	err = json.Unmarshal([]byte(resp), &token)
	if err != nil {
		return "", err
	}

	return token.RequestToken, err
}

func getDTPollResults(dtURL string, requestToken string, accessToken string) (respBody string, error error) {
	var dtPollRes DTLogsPollResult
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

	for {
		resp, err := requester.send()
		if err != nil {
			return "", err
		}

		err = json.Unmarshal([]byte(resp), &dtPollRes)
		if err != nil {
			return "", err
		}

		if dtPollRes.State == "RUNNING" {
			continue
		}

		if dtPollRes.State == "SUCCEEDED" {
			return resp, nil
		}

		if dtPollRes.State != "RUNNING" && dtPollRes.State != "SUCCEEDED" {
			return "", fmt.Errorf("query failed")
		}
	}
}

// getDocumentIDByNameAndType searches using the dynatrace document API using a filter that
// checks for an exact match of both name and type. It will return the id of the document
// found, unless it find zero or multiple in which case it will return an error
func getDocumentIDByNameAndType(dtURL string, accessToken string, docName string, docType string) (string, error) {
	dtDashFilter := "name == '" + docName + "' and type == '" + docType + "'"
	parameters := url.Values{
		"filter": {dtDashFilter},
	}.Encode()

	requester := Requester{
		method: http.MethodGet,
		url:    dtURL + "platform/document/v1/documents?" + parameters,
		headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + accessToken,
		},
		successCode: http.StatusOK,
	}

	result, err := requester.send()
	if err != nil {
		return "", fmt.Errorf("could not search for dashboard: %w", err)
	}

	var dtDocResult DTDocumentResult
	err = json.Unmarshal([]byte(result), &dtDocResult)
	if err != nil {
		return "", fmt.Errorf("response in incorrect format")
	}

	docCount := len(dtDocResult.Documents)
	if docCount == 0 {
		return "", fmt.Errorf("dashboard not found")
	}
	if docCount > 1 {
		return "", fmt.Errorf("dashboard name was ambiguous, %d dashboards found", docCount)
	}

	dtDashboard := dtDocResult.Documents[0]

	return dtDashboard.Id, nil
}

func getLogs(dtURL string, accessToken string, requestToken string, dumpWriter io.Writer) error {
	resp, err := getDTPollResults(dtURL, requestToken, accessToken)
	if err != nil {
		return err
	}

	var dtPollRes DTLogsPollResult
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

func getEvents(dtURL string, accessToken string, requestToken string, dumpWriter io.Writer) error {
	resp, err := getDTPollResults(dtURL, requestToken, accessToken)
	if err != nil {
		return err
	}

	var dtPollRes DTEventsPollResult
	err = json.Unmarshal([]byte(resp), &dtPollRes)
	if err != nil {
		return err
	}

	for _, result := range dtPollRes.Result.Records {
		if dumpWriter != nil {
			dumpWriter.Write([]byte(fmt.Sprintf("%s\n", result)))
		} else {
			fmt.Println(result)
		}
	}

	return nil
}
