package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	log "github.com/sirupsen/logrus"
)

// AccessTokenProvider returns a valid access token, refreshing if necessary.
type AccessTokenProvider interface {
	Token() (string, error)
}

// cachedTokenProvider caches an OAuth access token and transparently refreshes
// it when it is close to expiring.
type cachedTokenProvider struct {
	token     string
	expiresAt time.Time
	fetchFunc func() (string, int, error)
	margin    time.Duration
}

// newCachedTokenProvider creates a provider that calls fetchFunc to obtain a
// new token. fetchFunc must return the token string and its lifetime in seconds.
func newCachedTokenProvider(fetchFunc func() (string, int, error)) *cachedTokenProvider {
	return &cachedTokenProvider{
		fetchFunc: fetchFunc,
		margin:    30 * time.Second,
	}
}

// Token returns a valid access token, refreshing it if necessary.
func (p *cachedTokenProvider) Token() (string, error) {
	if p.token != "" && time.Now().Before(p.expiresAt.Add(-p.margin)) {
		return p.token, nil
	}

	token, expiresIn, err := p.fetchFunc()
	if err != nil {
		return "", err
	}

	p.token = token
	p.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return p.token, nil
}

// GetScopedAccessToken gets an access token using the vault path in the configuration key specified
// It will request any scopes listed in the scopes string
func GetScopedAccessToken(authUrl, vaultConfigKey string, scopes string) (string, error) {
	clientId, clientSecret, err := GetCredsFromVault(vaultConfigKey)
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

// GetScopedTokenProvider returns an AccessTokenProvider that fetches tokens
// using the vault path in the specified configuration key.
func GetScopedTokenProvider(authUrl, vaultConfigKey string, scopes string) (AccessTokenProvider, error) {
	clientId, clientSecret, err := GetCredsFromVault(vaultConfigKey)
	if err != nil {
		return nil, err
	}

	fetchFunc := func() (string, int, error) {
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
			return "", 0, err
		}

		var respObj map[string]interface{}
		err = json.Unmarshal([]byte(resp), &respObj)
		if err != nil {
			return "", 0, err
		}

		token, ok := respObj["access_token"].(string)
		if !ok {
			return "", 0, fmt.Errorf("access token not present in response")
		}

		expiresIn := 300 // default to 5 minutes if not present
		if v, ok := respObj["expires_in"].(float64); ok {
			expiresIn = int(v)
		}

		log.Infoln("Successfully authenticated")

		return token, expiresIn, nil
	}

	return newCachedTokenProvider(fetchFunc), nil
}
