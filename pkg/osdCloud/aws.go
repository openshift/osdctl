package osdCloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awsSdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/sts"

	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/viper"
)

const (
	RhSreCcsAccessRolename        = "RH-SRE-CCS-Access"
	RhTechnicalSupportAccess      = "RH-Technical-Support-Access"
	OrganizationAccountAccessRole = "OrganizationAccountAccessRole"
	ProdJumproleConfigKey         = "prod_jumprole_account_id"
	StageJumproleConfigKey        = "stage_jumprole_account_id"
)

// Uses the provided IAM Client to try and assume OrganizationAccountAccessRole for the given AWS Account
// This only works when the provided client is a user from the root account of an organization and the AWS account provided is a linked accounts within that organization
func GenerateOrganizationAccountAccessCredentials(client aws.Client, accountId, sessionName, partition string) (*sts.Credentials, error) {

	roleArnString := aws.GenerateRoleARN(accountId, "OrganizationAccountAccessRole")

	targetRoleArn, err := arn.Parse(roleArnString)
	if err != nil {
		return nil, err
	}

	targetRoleArn.Partition = partition

	assumeRoleOutput, err := client.AssumeRole(
		&sts.AssumeRoleInput{
			RoleArn:         awsSdk.String(targetRoleArn.String()),
			RoleSessionName: awsSdk.String(sessionName),
		},
	)
	if err != nil {
		return nil, err
	}

	return assumeRoleOutput.Credentials, nil

}

// Uses the provided IAM Client to perform the Assume Role chain needed to get to a cluster's Support Role
func GenerateSupportRoleCredentials(client aws.Client, awsAccountID, region, sessionName, targetRole string) (*sts.Credentials, error) {

	// Perform the Assume Role chain to get the jump
	jumpRoleCreds, err := GenerateJumpRoleCredentials(client, awsAccountID, region, sessionName)
	if err != nil {
		return nil, err
	}

	// Build client for jump role needed for the last step
	jumpRoleClient, err := aws.NewAwsClientWithInput(
		&aws.AwsClientInput{
			AccessKeyID:     *jumpRoleCreds.AccessKeyId,
			SecretAccessKey: *jumpRoleCreds.SecretAccessKey,
			SessionToken:    *jumpRoleCreds.SessionToken,
			Region:          *awsSdk.String(region),
		},
	)
	if err != nil {
		return nil, err
	}

	// Assume target ManagedOpenShift-Support role in the cluster's AWS Account
	targetAssumeRoleOutput, err := jumpRoleClient.AssumeRole(
		&sts.AssumeRoleInput{
			RoleArn:         awsSdk.String(targetRole),
			RoleSessionName: awsSdk.String(sessionName),
		},
	)
	if err != nil {
		return nil, err
	}

	return targetAssumeRoleOutput.Credentials, nil
}

// Preforms the Assume Role chain from IAM User to the Jump role
// This sequence stays within the Red Hat account boundary, so a failure here indicates an internal misconfiguration
func GenerateJumpRoleCredentials(client aws.Client, awsAccountID, region, sessionName string) (*sts.Credentials, error) {

	callerIdentityOutput, err := client.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, err
	}

	sreUserArn, err := arn.Parse(*callerIdentityOutput.Arn)
	if err != nil {
		return nil, err
	}

	// Assume RH-SRE-CCS-Access role
	sreCcsAccessRoleArn := aws.GenerateRoleARN(sreUserArn.AccountID, RhSreCcsAccessRolename)
	sreCcsAccessAssumeRoleOutput, err := client.AssumeRole(
		&sts.AssumeRoleInput{
			RoleArn:         awsSdk.String(sreCcsAccessRoleArn),
			RoleSessionName: awsSdk.String(sessionName),
		},
	)
	if err != nil {
		return nil, err
	}

	// Build client for RH-SRE-CCS-Access role
	sreCcsAccessRoleClient, err := aws.NewAwsClientWithInput(
		&aws.AwsClientInput{
			AccessKeyID:     *sreCcsAccessAssumeRoleOutput.Credentials.AccessKeyId,
			SecretAccessKey: *sreCcsAccessAssumeRoleOutput.Credentials.SecretAccessKey,
			SessionToken:    *sreCcsAccessAssumeRoleOutput.Credentials.SessionToken,
			Region:          *awsSdk.String(region),
		},
	)
	if err != nil {
		return nil, err
	}

	jumpRoleKey := ProdJumproleConfigKey
	currentEnv := utils.GetCurrentOCMEnv(utils.CreateConnection())
	if currentEnv == "stage" || currentEnv == "integration" {
		jumpRoleKey = StageJumproleConfigKey
	}

	if !viper.IsSet(jumpRoleKey) {
		return nil, fmt.Errorf("key %s is not set in config file", jumpRoleKey)
	}
	// Assume jump role
	// This will be different between stage and prod. There's probably a better way to do this that isn't hardcoding
	jumproleAccountID := viper.GetString(jumpRoleKey)

	jumpRoleArn := aws.GenerateRoleARN(jumproleAccountID, RhTechnicalSupportAccess)
	jumpAssumeRoleOutput, err := sreCcsAccessRoleClient.AssumeRole(
		&sts.AssumeRoleInput{
			RoleArn:         awsSdk.String(jumpRoleArn),
			RoleSessionName: awsSdk.String(sessionName),
		},
	)
	if err != nil {
		return nil, err
	}

	return jumpAssumeRoleOutput.Credentials, nil

}

// Uses the current IAM ARN to generate a role name. This should end up being RH-SRE-$kerberosID
func GenerateRoleSessionName(client aws.Client) (string, error) {

	callerIdentityOutput, err := client.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}

	roleArn, err := arn.Parse(awsSdk.StringValue(callerIdentityOutput.Arn))
	if err != nil {
		return "", err
	}

	splitArn := strings.Split(roleArn.Resource, "/")
	username := splitArn[1]

	return fmt.Sprintf("RH-SRE-%s", username), nil
}

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
	input, err := GetAWSClientInputFromBackplane(clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve AWS credentials from backplane: %w", err)
	}

	return aws.NewAwsClientWithInput(input)
}

// CreateAWSV2Config creates an aws-sdk-go-v2 config via Backplane given a cluster id
func CreateAWSV2Config(ctx context.Context, clusterID string) (awsv2.Config, error) {
	input, err := GetAWSClientInputFromBackplane(clusterID)
	if err != nil {
		return awsv2.Config{}, fmt.Errorf("failed to retrieve AWS credentials from backplane, ensure there is an active backplane tunnel: %s", err)
	}

	return config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(
			awsv2.NewCredentialsCache(
				credentials.NewStaticCredentialsProvider(
					input.AccessKeyID,
					input.SecretAccessKey,
					input.SessionToken,
				),
			),
		),
		config.WithRegion(input.Region),
	)
}

// GetAWSClientInputFromBackplane sets up AWS credentials via backplane-api given a cluster id
func GetAWSClientInputFromBackplane(clusterID string) (*aws.AwsClientInput, error) {
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

	return &aws.AwsClientInput{
		AccessKeyID:     awsCredentials.AccessKeyId,
		SecretAccessKey: awsCredentials.SecretAccessKey,
		SessionToken:    awsCredentials.SessionToken,
		Region:          *cloudCredentials.Region,
	}, nil
}

func GenerateCCSClusterAWSClient(ocmClient *sdk.Connection, awsClient aws.Client, clusterID string, clusterRegion string, partition string, sessionName string) (aws.Client, error) {
	// Determine the right jump role
	targetRoleArnString, err := utils.GetSupportRoleArnForCluster(ocmClient, clusterID)
	if err != nil {
		return nil, err
	}

	targetRoleArn, err := arn.Parse(targetRoleArnString)
	if err != nil {
		return nil, err
	}

	targetRoleArn.Partition = partition

	// Start the jump role chain. Result should be credentials for the ManagedOpenShift Support role for the target cluster
	assumedRoleCreds, err := GenerateSupportRoleCredentials(awsClient, targetRoleArn.AccountID, clusterRegion, sessionName, targetRoleArn.String())
	if err != nil {
		return nil, err
	}

	awsClientCCS, err := aws.NewAwsClientWithInput(&aws.AwsClientInput{
		AccessKeyID:     *assumedRoleCreds.AccessKeyId,
		SecretAccessKey: *assumedRoleCreds.SecretAccessKey,
		SessionToken:    *assumedRoleCreds.SessionToken,
		Region:          clusterRegion,
	})
	if err != nil {
		return nil, err
	}
	return awsClientCCS, nil
}

func GenerateNonCCSClusterAWSClient(ocmClient *sdk.Connection, awsClient aws.Client, clusterID string, clusterRegion string, partition string, sessionName string) (aws.Client, error) {
	accountID, err := utils.GetAWSAccountIdForCluster(ocmClient, clusterID)
	if err != nil {
		return nil, err
	}
	// If the cluster is non-CCS, or an AWS Account ID was provided with -i, try and use OrganizationAccountAccessRole
	assumedRoleCreds, err := GenerateOrganizationAccountAccessCredentials(awsClient, accountID, sessionName, partition)
	if err != nil {
		fmt.Printf("Could not build AWS Client for OrganizationAccountAccessRole: %s\n", err)
		return nil, err
	}

	awsClientNonCCS, err := aws.NewAwsClientWithInput(&aws.AwsClientInput{
		AccessKeyID:     *assumedRoleCreds.AccessKeyId,
		SecretAccessKey: *assumedRoleCreds.SecretAccessKey,
		SessionToken:    *assumedRoleCreds.SessionToken,
		Region:          clusterRegion,
	})
	if err != nil {
		return nil, err
	}
	return awsClientNonCCS, nil
}

// GenerateAWSClientForCluster generates an AWS client given an OCM cluster id and AWS profile name.
// If an AWS profile name is not specified, this function will also read the AWS_PROFILE environment
// variable or use the default AWS profile.
func GenerateAWSClientForCluster(awsProfile string, clusterID string) (aws.Client, error) {
	ocmClient := utils.CreateConnection()
	defer ocmClient.Close()

	cluster, err := utils.GetClusterAnyStatus(ocmClient, clusterID)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	clusterRegion := cluster.Region().ID()
	internalClusterId := cluster.ID()

	// Builds the base client using the provided creds (via profile or env vars)
	awsClient, err := aws.NewAwsClient(awsProfile, clusterRegion, "")
	if err != nil {
		fmt.Printf("Could not build AWS Client: %s\n", err)
		return nil, err
	}

	// Get the right partition for the final ARN
	partition, err := aws.GetAwsPartition(awsClient)
	if err != nil {
		return nil, err
	}

	// Generate a session name using the SRE's kerberos ID
	sessionName, err := GenerateRoleSessionName(awsClient)
	if err != nil {
		fmt.Printf("Could not generate Session Name: %s\n", err)
		return nil, err
	}

	if cluster.CCS().Enabled() {
		awsClient, err = GenerateCCSClusterAWSClient(ocmClient, awsClient, internalClusterId, clusterRegion, partition, sessionName)
	} else {
		awsClient, err = GenerateNonCCSClusterAWSClient(ocmClient, awsClient, internalClusterId, clusterRegion, partition, sessionName)
	}

	return awsClient, err
}
