package aws

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
)

const (
	defaultIssuer = "Red Hat SRE"

	PartitionID      = "aws"        // AWS Standard partition.
	UsGovPartitionID = "aws-us-gov" // AWS GovCloud (US) partition.
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

// GetAwsPartition uses sts GetCallerIdentity to determine the AWS partition we're in
func GetAwsPartition(awsClient Client) (string, error) {
	callerIdentityOutput, err := awsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}
	userArn, err := arn.Parse(*callerIdentityOutput.Arn)
	if err != nil {
		return "", err
	}

	return userArn.Partition, nil
}

// GetFederationEndpointUrl returns the default AWS Sign-In Federation endpoint for a given partition
func GetFederationEndpointUrl(partition string) (string, error) {
	switch partition {
	case PartitionID:
		// us-east-1 endpoint
		return "https://signin.aws.amazon.com/federation", nil
	case UsGovPartitionID:
		// us-gov-west-1 endpoint
		return "https://signin.amazonaws-us-gov.com/federation", nil
	default:
		return "", fmt.Errorf("invalid partition %s", partition)
	}
}

// GetConsoleUrl returns the default AWS Console base URL for a given partition
func GetConsoleUrl(partition string) (string, error) {
	switch partition {
	case PartitionID:
		// us-east-1 endpoint
		return "https://console.aws.amazon.com/", nil
	case UsGovPartitionID:
		// us-gov-west-1 endpoint
		return "https://console.amazonaws-us-gov.com/", nil
	default:
		return "", fmt.Errorf("invalid partition %s", partition)
	}
}

// RequestSignInToken makes an HTTP request to retrieve an AWS Sign-In Token via the AWS Federation endpoint
func RequestSignInToken(awsClient Client, durationSeconds *int32, sessionName, roleArn *string) (string, error) {
	credentials, err := GetAssumeRoleCredentials(awsClient, durationSeconds, sessionName, roleArn)
	if err != nil {
		return "", err
	}

	partition, err := GetAwsPartition(awsClient)
	if err != nil {
		return "", err
	}

	federationEndpointUrl, err := GetFederationEndpointUrl(partition)
	if err != nil {
		return "", err
	}

	signInToken, err := getSignInToken(federationEndpointUrl, credentials)
	if err != nil {
		return "", err
	}

	if signInToken == "" {
		return "", fmt.Errorf("sign-in token is empty")
	}

	signedFederationURL, err := formatSignInURL(partition, signInToken)
	if err != nil {
		return "", err
	}

	// Return Sign-In Token
	return signedFederationURL.String(), nil
}

// GetAssumeRoleCredentials gets the assume role credentials from AWS.
func GetAssumeRoleCredentials(awsClient Client, durationSeconds *int32, roleSessionName, roleArn *string) (*types.Credentials, error) {
	assumeRoleOutput, err := awsClient.AssumeRole(&sts.AssumeRoleInput{
		DurationSeconds: durationSeconds,
		RoleSessionName: roleSessionName,
		RoleArn:         roleArn,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to assume role: %v", err)
	}

	if assumeRoleOutput == nil {
		log.Printf("Get assume role output nil %v", awsv1alpha1.ErrFederationTokenOutputNil)
		return nil, awsv1alpha1.ErrFederationTokenOutputNil
	}

	return assumeRoleOutput.Credentials, nil
}

// getSignInToken makes a request to the federation endpoint to sign signin token
// Takes a logger, the base url, and the federation token to sign with
func getSignInToken(baseURL string, creds *types.Credentials) (string, error) {
	credsPayload := sessionPayload{
		SessionID:    *creds.AccessKeyId,
		SessionKey:   *creds.SecretAccessKey,
		SessionToken: *creds.SessionToken,
	}

	data, err := json.Marshal(credsPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal credentials to json: %v", err)
	}

	token, err := requestSignedURL(baseURL, data)
	if err != nil {
		return "", err
	}

	return token, nil
}

// requestSignedURL makes an HTTP call to the baseFederationURL to retrieve a signed federated URL for web console login
// Takes a logger, and the base URL
func requestSignedURL(baseUrl string, jsonCredentials []byte) (string, error) {
	// Build URL to request SignIn Token via Federation end point
	baseFederationURL, err := url.Parse(baseUrl)
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
		return "", fmt.Errorf("failed to request SignIn token from %s: %v", baseFederationURL, err)
	}

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to request Sign-In token from: %s, status code %d", baseFederationURL, res.StatusCode)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	var resp awsSignInTokenResponse
	if err = json.Unmarshal(body, &resp); err != nil {
		return "", err
	}

	return resp.SigninToken, nil
}

// formatSignInURL builds and format the Sign-In URL
func formatSignInURL(partition, signInToken string) (*url.URL, error) {
	federationEndpointUrl, err := GetFederationEndpointUrl(partition)
	if err != nil {
		return nil, err
	}

	consoleUrl, err := GetConsoleUrl(partition)
	if err != nil {
		return nil, err
	}

	signInFederationURL, err := url.Parse(federationEndpointUrl)
	if err != nil {
		return nil, err
	}

	signinParams := url.Values{}
	signinParams.Add("Action", "login")
	signinParams.Add("Destination", consoleUrl)
	signinParams.Add("Issuer", defaultIssuer)
	signinParams.Add("SigninToken", signInToken)
	signInFederationURL.RawQuery = signinParams.Encode()

	return signInFederationURL, nil
}
