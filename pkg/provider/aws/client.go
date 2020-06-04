package aws

import (
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
)

// AwsClientInput input for new aws client
type AwsClientInput struct {
	AwsIDKey     string
	AwsAccessKey string
	AwsToken     string
	AwsRegion    string
}

// TODO: Add more methods when needed
type Client interface {
	//sts
	AssumeRole(*sts.AssumeRoleInput) (*sts.AssumeRoleOutput, error)
	GetCallerIdentity(*sts.GetCallerIdentityInput) (*sts.GetCallerIdentityOutput, error)
	GetFederationToken(input *sts.GetFederationTokenInput) (*sts.GetFederationTokenOutput, error)
}

type AwsClient struct {
	stsClient stsiface.STSAPI
}

// NewAwsClient creates an AWS client with credentials in the environment
func NewAwsClient(region string) (Client, error) {
	config := &aws.Config{
		Region: aws.String(region),
	}

	sess := session.Must(session.NewSession(config))
	_, err := sess.Config.Credentials.Get()

	if aerr, ok := err.(awserr.Error); ok {
		switch aerr.Code() {
		case "NoCredentialProviders":
			return nil, errors.Wrap(err, "Could not create AWS session")
		default:
			return nil, errors.Wrap(err, "Could not create AWS session")
		}
	}

	return &AwsClient{
		stsClient: sts.New(sess),
	}, nil
}

// NewAwsClientWithInput creates an AWS client with input credentials
func NewAwsClientWithInput(input *AwsClientInput) (Client, error) {
	config := &aws.Config{
		Credentials: credentials.NewStaticCredentials(input.AwsIDKey, input.AwsAccessKey, input.AwsToken),
		Region:      aws.String(input.AwsRegion),
	}

	s, err := session.NewSession(config)
	if err != nil {
		return nil, err
	}

	return &AwsClient{
		stsClient: sts.New(s),
	}, nil
}

func (c *AwsClient) AssumeRole(input *sts.AssumeRoleInput) (*sts.AssumeRoleOutput, error) {
	return c.stsClient.AssumeRole(input)
}

func (c *AwsClient) GetCallerIdentity(input *sts.GetCallerIdentityInput) (*sts.GetCallerIdentityOutput, error) {
	return c.stsClient.GetCallerIdentity(input)
}

func (c *AwsClient) GetFederationToken(input *sts.GetFederationTokenInput) (*sts.GetFederationTokenOutput, error) {
	GetFederationTokenOutput, err := c.stsClient.GetFederationToken(input)
	if GetFederationTokenOutput != nil {
		return GetFederationTokenOutput, err
	}
	return &sts.GetFederationTokenOutput{}, err
}
