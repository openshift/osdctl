package aws

// Generate client mocks for testing
//go:generate mockgen -source=client.go -package=mock -destination=mock/client.go

import (
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"

	"github.com/pkg/errors"
)

// AwsClientInput input for new aws client
type AwsClientInput struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
}

// TODO: Add more methods when needed
type Client interface {
	// sts
	AssumeRole(*sts.AssumeRoleInput) (*sts.AssumeRoleOutput, error)
	GetCallerIdentity(*sts.GetCallerIdentityInput) (*sts.GetCallerIdentityOutput, error)
	GetFederationToken(*sts.GetFederationTokenInput) (*sts.GetFederationTokenOutput, error)

	// S3
	ListBuckets(*s3.ListBucketsInput) (*s3.ListBucketsOutput, error)
	DeleteBucket(*s3.DeleteBucketInput) (*s3.DeleteBucketOutput, error)
	ListObjects(*s3.ListObjectsInput) (*s3.ListObjectsOutput, error)
	DeleteObjects(*s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error)

	//iam
	CreateAccessKey(*iam.CreateAccessKeyInput) (*iam.CreateAccessKeyOutput, error)
	DeleteAccessKey(*iam.DeleteAccessKeyInput) (*iam.DeleteAccessKeyOutput, error)
	ListAccessKeys(*iam.ListAccessKeysInput) (*iam.ListAccessKeysOutput, error)
	GetUser(*iam.GetUserInput) (*iam.GetUserOutput, error)
	CreateUser(*iam.CreateUserInput) (*iam.CreateUserOutput, error)
	ListUsers(*iam.ListUsersInput) (*iam.ListUsersOutput, error)
	AttachUserPolicy(*iam.AttachUserPolicyInput) (*iam.AttachUserPolicyOutput, error)
	CreatePolicy(*iam.CreatePolicyInput) (*iam.CreatePolicyOutput, error)
	DeletePolicy(*iam.DeletePolicyInput) (*iam.DeletePolicyOutput, error)
	AttachRolePolicy(*iam.AttachRolePolicyInput) (*iam.AttachRolePolicyOutput, error)
	DetachRolePolicy(*iam.DetachRolePolicyInput) (*iam.DetachRolePolicyOutput, error)
	ListAttachedRolePolicies(*iam.ListAttachedRolePoliciesInput) (*iam.ListAttachedRolePoliciesOutput, error)

	// Organizations and Cost Explorer
	GetOrg() *organizations.Organizations
	GetCE() *costexplorer.CostExplorer
}

type AwsClient struct {
	iamClient iamiface.IAMAPI
	stsClient stsiface.STSAPI
	s3Client  s3iface.S3API
	orgClient *organizations.Organizations
	ceClient  *costexplorer.CostExplorer
}

type OrganizationsClient interface {
	ListAccountsForParent(input *organizations.ListAccountsForParentInput) (*organizations.ListAccountsForParentOutput, error)
	ListOrganizationalUnitsForParent(input *organizations.ListOrganizationalUnitsForParentInput) (*organizations.ListOrganizationalUnitsForParentOutput, error)
}

type CostExplorerClient interface {
	GetCostAndUsage(input *costexplorer.GetCostAndUsageInput) (*costexplorer.GetCostAndUsageOutput, error)
	CreateCostCategoryDefinition(input *costexplorer.CreateCostCategoryDefinitionInput) (*costexplorer.CreateCostCategoryDefinitionOutput, error)
	ListCostCategoryDefinitions(input *costexplorer.ListCostCategoryDefinitionsInput) (*costexplorer.ListCostCategoryDefinitionsOutput, error)
}

// NewAwsClient creates an AWS client with credentials in the environment
func NewAwsClient(profile, region, configFile string) (Client, error) {
	opt := session.Options{
		Config: aws.Config{
			Region: aws.String(region),
		},
		Profile: profile,
	}

	// only set config file if it is not empty
	if configFile != "" {
		absCfgPath, err := filepath.Abs(configFile)
		if err != nil {
			return nil, err
		}
		opt.SharedConfigFiles = []string{absCfgPath}
	}

	sess := session.Must(session.NewSessionWithOptions(opt))
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
		iamClient: iam.New(sess),
		stsClient: sts.New(sess),
		s3Client:  s3.New(sess),
		orgClient: organizations.New(sess),
		ceClient:  costexplorer.New(sess),
	}, nil
}

// NewAwsClientWithInput creates an AWS client with input credentials
func NewAwsClientWithInput(input *AwsClientInput) (Client, error) {
	config := &aws.Config{
		Credentials: credentials.NewStaticCredentials(input.AccessKeyID, input.SecretAccessKey, input.SessionToken),
		Region:      aws.String(input.Region),
	}

	s, err := session.NewSession(config)
	if err != nil {
		return nil, err
	}

	return &AwsClient{
		iamClient: iam.New(s),
		stsClient: sts.New(s),
		s3Client:  s3.New(s),
		orgClient: organizations.New(s),
		ceClient:  costexplorer.New(s),
	}, nil
}

func (c *AwsClient) AssumeRole(input *sts.AssumeRoleInput) (*sts.AssumeRoleOutput, error) {
	return c.stsClient.AssumeRole(input)
}

func (c *AwsClient) GetCallerIdentity(input *sts.GetCallerIdentityInput) (*sts.GetCallerIdentityOutput, error) {
	return c.stsClient.GetCallerIdentity(input)
}

func (c *AwsClient) GetFederationToken(input *sts.GetFederationTokenInput) (*sts.GetFederationTokenOutput, error) {
	return c.stsClient.GetFederationToken(input)
}

func (c *AwsClient) ListBuckets(input *s3.ListBucketsInput) (*s3.ListBucketsOutput, error) {
	return c.s3Client.ListBuckets(input)
}

func (c *AwsClient) DeleteBucket(input *s3.DeleteBucketInput) (*s3.DeleteBucketOutput, error) {
	return c.s3Client.DeleteBucket(input)
}

func (c *AwsClient) ListObjects(input *s3.ListObjectsInput) (*s3.ListObjectsOutput, error) {
	return c.s3Client.ListObjects(input)
}

func (c *AwsClient) DeleteObjects(input *s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error) {
	return c.s3Client.DeleteObjects(input)
}

func (c *AwsClient) CreateAccessKey(input *iam.CreateAccessKeyInput) (*iam.CreateAccessKeyOutput, error) {
	return c.iamClient.CreateAccessKey(input)
}

func (c *AwsClient) DeleteAccessKey(input *iam.DeleteAccessKeyInput) (*iam.DeleteAccessKeyOutput, error) {
	return c.iamClient.DeleteAccessKey(input)
}

func (c *AwsClient) ListAccessKeys(input *iam.ListAccessKeysInput) (*iam.ListAccessKeysOutput, error) {
	return c.iamClient.ListAccessKeys(input)
}

func (c *AwsClient) GetUser(input *iam.GetUserInput) (*iam.GetUserOutput, error) {
	return c.iamClient.GetUser(input)
}

func (c *AwsClient) CreateUser(input *iam.CreateUserInput) (*iam.CreateUserOutput, error) {
	return c.iamClient.CreateUser(input)
}

func (c *AwsClient) ListUsers(input *iam.ListUsersInput) (*iam.ListUsersOutput, error) {
	return c.iamClient.ListUsers(input)
}

func (c *AwsClient) AttachUserPolicy(input *iam.AttachUserPolicyInput) (*iam.AttachUserPolicyOutput, error) {
	return c.iamClient.AttachUserPolicy(input)
}

func (c *AwsClient) CreatePolicy(input *iam.CreatePolicyInput) (*iam.CreatePolicyOutput, error) {
	return c.iamClient.CreatePolicy(input)
}

func (c *AwsClient) DeletePolicy(input *iam.DeletePolicyInput) (*iam.DeletePolicyOutput, error) {
	return c.iamClient.DeletePolicy(input)
}

func (c *AwsClient) AttachRolePolicy(input *iam.AttachRolePolicyInput) (*iam.AttachRolePolicyOutput, error) {
	return c.iamClient.AttachRolePolicy(input)
}

func (c *AwsClient) DetachRolePolicy(input *iam.DetachRolePolicyInput) (*iam.DetachRolePolicyOutput, error) {
	return c.iamClient.DetachRolePolicy(input)
}

func (c *AwsClient) ListAttachedRolePolicies(input *iam.ListAttachedRolePoliciesInput) (*iam.ListAttachedRolePoliciesOutput, error) {
	return c.iamClient.ListAttachedRolePolicies(input)
}

func (c *AwsClient) GetOrg() *organizations.Organizations {
	return c.orgClient
}

func (c *AwsClient) GetCE() *costexplorer.CostExplorer {
	return c.ceClient
}
