package aws

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"k8s.io/klog"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go/service/sts"
	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
)

const (
	// AWS Federation URL
	federationEndpointURL = "https://signin.aws.amazon.com/federation"

	// AWS Console URL
	awsConsoleURL = "https://console.aws.amazon.com/"

	// default issuer name
	defaultIssuer = "Red Hat SRE"
)

// Type for JSON response from Federation end point
type awsSignInTokenResponse struct {
	SigninToken string
}

type sessionPayload struct {
	SessionID    string `json:"sessionId"`
	SessionKey   string `json:"sessionKey"`
	SessionToken string `json:"sessionToken"`
}

// RequestSignInToken makes a HTTP request to retrieve an AWS SignIn Token
// via the AWS Federation endpoint
func RequestSignInToken(awsClient Client, durationSeconds *int64, sessionName, roleArn *string) (string, error) {
	credentials, err := getAssumeRoleCredentials(awsClient, durationSeconds, sessionName, roleArn)
	if err != nil {
		return "", err
	}

	signInToken, err := getSignInToken(federationEndpointURL, credentials)
	if err != nil {
		return "", err
	}

	if signInToken == "" {
		return "", errors.New("SigninToken is empty")
	}

	signedFederationURL, err := formatSignInURL(signInToken)
	if err != nil {
		return "", err
	}

	// Return Signin Token
	return signedFederationURL.String(), nil
}

// getAssumeRoleCredentials gets the Federation Token from AWS.
func getAssumeRoleCredentials(awsClient Client, durationSeconds *int64, roleSessionName, roleArn *string) (*sts.Credentials, error) {
	assumeRoleOutput, err := awsClient.AssumeRole(&sts.AssumeRoleInput{
		DurationSeconds: durationSeconds,
		RoleSessionName: roleSessionName,
		RoleArn:         roleArn,
	})
	if err != nil {
		// Get error details
		klog.Errorf("Failed to assume role: %v", err)

		return nil, err
	}

	if assumeRoleOutput == nil {
		klog.Errorf("Get assume role output nil %v", awsv1alpha1.ErrFederationTokenOutputNil)
		return nil, awsv1alpha1.ErrFederationTokenOutputNil
	}

	return assumeRoleOutput.Credentials, nil
}

// getSignInToken makes a request to the federation endpoint to sign signin token
// Takes a logger, the base url, and the federation token to sign with
func getSignInToken(baseURL string, creds *sts.Credentials) (string, error) {
	credsPayload := sessionPayload{
		SessionID:    *creds.AccessKeyId,
		SessionKey:   *creds.SecretAccessKey,
		SessionToken: *creds.SessionToken,
	}

	data, err := json.Marshal(credsPayload)
	if err != nil {
		klog.Errorf("Failed to marshal credentials to json %v", err)
		return "", err
	}

	token, err := requestSignedURL(baseURL, data)
	if err != nil {
		return "", err
	}

	return token, nil
}

// requestSignedURL makes a HTTP call to the baseFederationURL to retrieve a signed federated URL for web console login
// Takes a logger, and the base URL
func requestSignedURL(baseURL string, jsonCredentials []byte) (string, error) {
	// Build URL to request SignIn Token via Federation end point
	baseFederationURL, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	federationParams := url.Values{}
	federationParams.Add("Action", "getSigninToken")
	federationParams.Add("SessionType", "json")
	federationParams.Add("Session", string(jsonCredentials))

	baseFederationURL.RawQuery = federationParams.Encode()

	// Make HTTP request to retrieve Federated SignIn Token
	res, err := http.Get(baseFederationURL.String())
	if err != nil {
		klog.Errorf("Failed to request Signin token from: %s, %v", baseFederationURL, err)
		return "", err
	}

	if res.StatusCode/100 != 2 {
		klog.Errorf("Failed to request Signin token from: %s, status code %d", baseFederationURL, res.StatusCode)
		return "", fmt.Errorf("bad response code %d", res.StatusCode)
	}

	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		klog.Errorf("Failed to read response body %v", err)
		return "", err
	}

	var resp awsSignInTokenResponse
	if err = json.Unmarshal(body, &resp); err != nil {
		return "", err
	}

	return resp.SigninToken, nil
}

// formatSignInURL build and format the signIn URL to be used in the secret
func formatSignInURL(signInToken string) (*url.URL, error) {
	issuer := defaultIssuer

	signInFederationURL, err := url.Parse(federationEndpointURL)
	if err != nil {
		return nil, err
	}

	signinParams := url.Values{}

	signinParams.Add("Action", "login")
	signinParams.Add("Destination", awsConsoleURL)
	signinParams.Add("Issuer", issuer)
	signinParams.Add("SigninToken", signInToken)

	signInFederationURL.RawQuery = signinParams.Encode()

	return signInFederationURL, nil
}
