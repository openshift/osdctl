package clustercloud

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
)

type cloudCredentialsResponse struct {
	// ClusterID
	ClusterID string `json:"clusterID"`

	// Link to the console, optional
	ConsoleLink *string `json:"consoleLink,omitempty"`

	// Cloud credentials, optional
	Credentials *string `json:"credentials,omitempty"`

	// Region, optional
	Region *string `json:"region,omitempty"`
}

type awsCredentialsResponse struct {
	AccessKeyId     string `json:"AccessKeyId" yaml:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey" yaml:"SecretAccessKey"`
	SessionToken    string `json:"SessionToken" yaml:"SessionToken"`
	Region          string `json:"Region" yaml:"Region"`
	Expiration      string `json:"Expiration" yaml:"Expiration"`
}

// Creates an AWS client based on a clusterid
// Requires previous log on to the correct api server via ocm login
// and tunneling to the backplane
func CreateAWSClient(clusterID string) (aws.Client, error) {
	token, err := utils.GetOCMApiServerToken()

	getUrl, err := utils.GetBackplaneURL(clusterID)
	if err != nil {
		return nil, fmt.Errorf("Unable to retrieve backplane URL for cluster %s: %s", clusterID, err)
	}

	client := http.Client{}

	request, _ := http.NewRequest("GET", getUrl, nil)
	if err != nil {
		return nil, err
	}

	request.Header.Set("Authorization", "Bearer "+*token)
	request.Header.Set("User-Agent", "osdctl")

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	var cloudCredentials cloudCredentialsResponse
	var awsCredentials awsCredentialsResponse
	if resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		cloudCredentials = cloudCredentialsResponse{}

		err = json.Unmarshal(bodyBytes, &cloudCredentials)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshal cloud credentials: %s", err)
		}

		err = json.Unmarshal([]byte(*cloudCredentials.Credentials), &awsCredentials)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshal aws credentials: %s", err)
		}
	}

	input := aws.AwsClientInput{AccessKeyID: awsCredentials.AccessKeyId, SecretAccessKey: awsCredentials.SecretAccessKey, SessionToken: awsCredentials.SessionToken, Region: *cloudCredentials.Region}

	return aws.NewAwsClientWithInput(&input)
}
