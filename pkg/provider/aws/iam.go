package aws

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"k8s.io/klog"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go/aws/awserr"
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

// RequestSignInToken makes a HTTP request to retrieve an AWS SignIn Token
// via the AWS Federation endpoint
func RequestSignInToken(awsClient Client, durationSeconds *int64, sessionName, roleArn *string) (string, error) {
	credentials, err := getAssumeRoleCredentials(awsClient, durationSeconds, sessionName, roleArn)
	if err != nil {
		return "", err
	}

	signInToken, err := getSignInToken(credentials)
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

// getFederationToken gets the Federation Token from AWS.
func getFederationToken(awsClient Client, DurationSeconds *int64, FederatedUserName *string, PolicyArns []*sts.PolicyDescriptorType) (*sts.Credentials, error) {
	getFederationTokenInput := &sts.GetFederationTokenInput{
		DurationSeconds: DurationSeconds,
		Name:            FederatedUserName,
		PolicyArns:      PolicyArns,
	}

	// Get Federated token credentials to build console URL
	GetFederationTokenOutput, err := awsClient.GetFederationToken(getFederationTokenInput)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			// Get error details
			klog.Errorf("Failed to get federation token: %s, %s %v", awsErr.Code(), awsErr.Message(), err)
			return nil, err
		}

		return nil, err
	}

	if GetFederationTokenOutput == nil {
		klog.Errorf("Get federation token nil %v", awsv1alpha1.ErrFederationTokenOutputNil)
		return nil, awsv1alpha1.ErrFederationTokenOutputNil
	}

	return GetFederationTokenOutput.Credentials, nil
}

// getAssumeRoleCredentials gets the Federation Token from AWS.
func getAssumeRoleCredentials(awsClient Client, durationSeconds *int64, roleSessionName, roleArn *string) (*sts.Credentials, error) {
	assumeRoleOutput, err := awsClient.AssumeRole(&sts.AssumeRoleInput{
		DurationSeconds: durationSeconds,
		RoleSessionName: roleSessionName,
		RoleArn:         roleArn,
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			// Get error details
			klog.Errorf("Failed to assume role: %s, %s %v", awsErr.Code(), awsErr.Message(), err)
			return nil, err
		}

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
func getSignInToken(creds *sts.Credentials) (string, error) {
	jsonCreds := map[string]string{
		"sessionId":    *creds.AccessKeyId,
		"sessionKey":   *creds.SecretAccessKey,
		"sessionToken": *creds.SessionToken,
	}

	data, err := json.Marshal(jsonCreds)
	if err != nil {
		klog.Errorf("Failed to marshal federation credentials %v", err)
		return "", err
	}

	token, err := requestSignedURL(data)
	if err != nil {
		return "", err
	}

	return token, nil
}

// requestSignedURL makes a HTTP call to the baseFederationURL to retrieve a signed federated URL for web console login
// Takes a logger, and the base URL
func requestSignedURL(jsonCredentials []byte) (string, error) {
	// Build URL to request SignIn Token via Federation end point
	baseFederationURL, err := url.Parse(federationEndpointURL)
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
