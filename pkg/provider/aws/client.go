package aws

// Generate client mocks for testing
//go:generate mockgen -source=client.go -package=mock -destination=mock/client.go

import (
	"fmt"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/costexplorer/costexploreriface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/aws/aws-sdk-go/service/organizations/organizationsiface"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi/resourcegroupstaggingapiiface"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/servicequotas"
	"github.com/aws/aws-sdk-go/service/servicequotas/servicequotasiface"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
	"k8s.io/klog/v2"
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
	DeleteLoginProfile(*iam.DeleteLoginProfileInput) (*iam.DeleteLoginProfileOutput, error)
	ListSigningCertificates(*iam.ListSigningCertificatesInput) (*iam.ListSigningCertificatesOutput, error)
	DeleteSigningCertificate(*iam.DeleteSigningCertificateInput) (*iam.DeleteSigningCertificateOutput, error)
	ListUserPolicies(*iam.ListUserPoliciesInput) (*iam.ListUserPoliciesOutput, error)
	ListPolicies(*iam.ListPoliciesInput) (*iam.ListPoliciesOutput, error)
	DeleteUserPolicy(*iam.DeleteUserPolicyInput) (*iam.DeleteUserPolicyOutput, error)
	ListAttachedUserPolicies(*iam.ListAttachedUserPoliciesInput) (*iam.ListAttachedUserPoliciesOutput, error)
	DetachUserPolicy(*iam.DetachUserPolicyInput) (*iam.DetachUserPolicyOutput, error)
	ListGroupsForUser(*iam.ListGroupsForUserInput) (*iam.ListGroupsForUserOutput, error)
	RemoveUserFromGroup(*iam.RemoveUserFromGroupInput) (*iam.RemoveUserFromGroupOutput, error)
	ListRoles(*iam.ListRolesInput) (*iam.ListRolesOutput, error)
	DeleteRole(*iam.DeleteRoleInput) (*iam.DeleteRoleOutput, error)
	DeleteUser(*iam.DeleteUserInput) (*iam.DeleteUserOutput, error)

	//ec2
	DescribeInstances(*ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error)

	// Service Quotas
	ListServiceQuotas(*servicequotas.ListServiceQuotasInput) (*servicequotas.ListServiceQuotasOutput, error)
	RequestServiceQuotaIncrease(*servicequotas.RequestServiceQuotaIncreaseInput) (*servicequotas.RequestServiceQuotaIncreaseOutput, error)

	// Organizations
	CreateAccount(input *organizations.CreateAccountInput) (*organizations.CreateAccountOutput, error)
	DescribeCreateAccountStatus(input *organizations.DescribeCreateAccountStatusInput) (*organizations.DescribeCreateAccountStatusOutput, error)
	ListAccounts(input *organizations.ListAccountsInput) (*organizations.ListAccountsOutput, error)
	ListParents(input *organizations.ListParentsInput) (*organizations.ListParentsOutput, error)
	ListRoots(input *organizations.ListRootsInput) (*organizations.ListRootsOutput, error)
	ListAccountsForParent(input *organizations.ListAccountsForParentInput) (*organizations.ListAccountsForParentOutput, error)
	ListOrganizationalUnitsForParent(input *organizations.ListOrganizationalUnitsForParentInput) (*organizations.ListOrganizationalUnitsForParentOutput, error)
	DescribeOrganizationalUnit(input *organizations.DescribeOrganizationalUnitInput) (*organizations.DescribeOrganizationalUnitOutput, error)
	TagResource(input *organizations.TagResourceInput) (*organizations.TagResourceOutput, error)
	UntagResource(input *organizations.UntagResourceInput) (*organizations.UntagResourceOutput, error)
	ListTagsForResource(input *organizations.ListTagsForResourceInput) (*organizations.ListTagsForResourceOutput, error)
	MoveAccount(input *organizations.MoveAccountInput) (*organizations.MoveAccountOutput, error)

	// Resources
	GetResources(input *resourcegroupstaggingapi.GetResourcesInput) (*resourcegroupstaggingapi.GetResourcesOutput, error)

	// Cost Explorer
	GetCostAndUsage(input *costexplorer.GetCostAndUsageInput) (*costexplorer.GetCostAndUsageOutput, error)
	CreateCostCategoryDefinition(input *costexplorer.CreateCostCategoryDefinitionInput) (*costexplorer.CreateCostCategoryDefinitionOutput, error)
	ListCostCategoryDefinitions(input *costexplorer.ListCostCategoryDefinitionsInput) (*costexplorer.ListCostCategoryDefinitionsOutput, error)
}

type AwsClient struct {
	iamClient           iamiface.IAMAPI
	ec2Client           ec2iface.EC2API
	stsClient           stsiface.STSAPI
	s3Client            s3iface.S3API
	servicequotasClient servicequotasiface.ServiceQuotasAPI
	orgClient           organizationsiface.OrganizationsAPI
	resClient           resourcegroupstaggingapiiface.ResourceGroupsTaggingAPIAPI
	ceClient            costexploreriface.CostExplorerAPI
}

func NewAwsSession(profile, region, configFile string) (*session.Session, error) {
	if profile == "" && configFile == "" {
		fmt.Println("Config file and profile are not provided. Reading from env vars.")
	}

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
			return nil, fmt.Errorf("could not load config file: %v", err)
		}
		opt.SharedConfigFiles = []string{absCfgPath}
	}

	sess := session.Must(session.NewSessionWithOptions(opt))
	if _, err := sess.Config.Credentials.Get(); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case "NoCredentialProviders":
				return nil, fmt.Errorf("could not create AWS session: %v", err)
			default:
				return nil, fmt.Errorf("could not create AWS session: %v", err)
			}
		}
	}

	return sess, nil
}

// NewAwsClient creates an AWS client with credentials in the environment
func NewAwsClient(profile, region, configFile string) (Client, error) {
	sess, err := NewAwsSession(profile, region, configFile)
	if err != nil {
		return nil, err
	}

	awsClient := &AwsClient{iamClient: iam.New(sess),
		ec2Client:           ec2.New(sess),
		stsClient:           sts.New(sess),
		s3Client:            s3.New(sess),
		servicequotasClient: servicequotas.New(sess),
		orgClient:           organizations.New(sess),
		ceClient:            costexplorer.New(sess),
		resClient:           resourcegroupstaggingapi.New(sess),
	}

	// Validate the creds
	if _, err := awsClient.GetCallerIdentity(nil); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case "InvalidClientTokenId":
				if region == endpoints.UsGovEast1RegionID || region == endpoints.UsGovWest1RegionID {
					return nil, fmt.Errorf("failed `aws sts get-caller-identity` validation: %v", err)
				}
				klog.Infoln("credentials provided invalid, trying GovCloud by setting region to us-gov-west-1.")
				return NewAwsClient(profile, endpoints.UsGovWest1RegionID, configFile)
			default:
				return nil, fmt.Errorf("failed `aws sts get-caller-identity` validation: %v", err)
			}
		}
	}

	return awsClient, nil
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
		iamClient:           iam.New(s),
		ec2Client:           ec2.New(s),
		stsClient:           sts.New(s),
		s3Client:            s3.New(s),
		servicequotasClient: servicequotas.New(s),
		orgClient:           organizations.New(s),
		ceClient:            costexplorer.New(s),
		resClient:           resourcegroupstaggingapi.New(s),
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

func (c *AwsClient) ListPolicies(input *iam.ListPoliciesInput) (*iam.ListPoliciesOutput, error) {
	return c.iamClient.ListPolicies(input)
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

func (c *AwsClient) DeleteLoginProfile(input *iam.DeleteLoginProfileInput) (*iam.DeleteLoginProfileOutput, error) {
	return c.iamClient.DeleteLoginProfile(input)
}

func (c *AwsClient) ListSigningCertificates(input *iam.ListSigningCertificatesInput) (*iam.ListSigningCertificatesOutput, error) {
	return c.iamClient.ListSigningCertificates(input)
}

func (c *AwsClient) DeleteSigningCertificate(input *iam.DeleteSigningCertificateInput) (*iam.DeleteSigningCertificateOutput, error) {
	return c.iamClient.DeleteSigningCertificate(input)
}

func (c *AwsClient) ListUserPolicies(input *iam.ListUserPoliciesInput) (*iam.ListUserPoliciesOutput, error) {
	return c.iamClient.ListUserPolicies(input)
}

func (c *AwsClient) DeleteUserPolicy(input *iam.DeleteUserPolicyInput) (*iam.DeleteUserPolicyOutput, error) {
	return c.iamClient.DeleteUserPolicy(input)
}

func (c *AwsClient) ListAttachedUserPolicies(input *iam.ListAttachedUserPoliciesInput) (*iam.ListAttachedUserPoliciesOutput, error) {
	return c.iamClient.ListAttachedUserPolicies(input)
}

func (c *AwsClient) DetachUserPolicy(input *iam.DetachUserPolicyInput) (*iam.DetachUserPolicyOutput, error) {
	return c.iamClient.DetachUserPolicy(input)
}

func (c *AwsClient) ListGroupsForUser(input *iam.ListGroupsForUserInput) (*iam.ListGroupsForUserOutput, error) {
	return c.iamClient.ListGroupsForUser(input)
}

func (c *AwsClient) RemoveUserFromGroup(input *iam.RemoveUserFromGroupInput) (*iam.RemoveUserFromGroupOutput, error) {
	return c.iamClient.RemoveUserFromGroup(input)
}

func (c *AwsClient) ListRoles(input *iam.ListRolesInput) (*iam.ListRolesOutput, error) {
	return c.iamClient.ListRoles(input)
}

func (c *AwsClient) DeleteRole(input *iam.DeleteRoleInput) (*iam.DeleteRoleOutput, error) {
	return c.iamClient.DeleteRole(input)
}

func (c *AwsClient) DeleteUser(input *iam.DeleteUserInput) (*iam.DeleteUserOutput, error) {
	return c.iamClient.DeleteUser(input)
}

func (c *AwsClient) ListAccounts(input *organizations.ListAccountsInput) (*organizations.ListAccountsOutput, error) {
	return c.orgClient.ListAccounts(input)
}

func (c *AwsClient) ListParents(input *organizations.ListParentsInput) (*organizations.ListParentsOutput, error) {
	return c.orgClient.ListParents(input)
}
func (c *AwsClient) ListRoots(input *organizations.ListRootsInput) (*organizations.ListRootsOutput, error) {
	return c.orgClient.ListRoots(input)
}
func (c *AwsClient) ListAccountsForParent(input *organizations.ListAccountsForParentInput) (*organizations.ListAccountsForParentOutput, error) {
	return c.orgClient.ListAccountsForParent(input)
}

func (c *AwsClient) ListServiceQuotas(input *servicequotas.ListServiceQuotasInput) (*servicequotas.ListServiceQuotasOutput, error) {
	return c.servicequotasClient.ListServiceQuotas(input)
}

func (c *AwsClient) RequestServiceQuotaIncrease(input *servicequotas.RequestServiceQuotaIncreaseInput) (*servicequotas.RequestServiceQuotaIncreaseOutput, error) {
	return c.servicequotasClient.RequestServiceQuotaIncrease(input)
}

func (c *AwsClient) CreateAccount(input *organizations.CreateAccountInput) (*organizations.CreateAccountOutput, error) {
	return c.orgClient.CreateAccount(input)
}

func (c *AwsClient) DescribeCreateAccountStatus(input *organizations.DescribeCreateAccountStatusInput) (*organizations.DescribeCreateAccountStatusOutput, error) {
	return c.orgClient.DescribeCreateAccountStatus(input)
}

func (c *AwsClient) ListOrganizationalUnitsForParent(input *organizations.ListOrganizationalUnitsForParentInput) (*organizations.ListOrganizationalUnitsForParentOutput, error) {
	return c.orgClient.ListOrganizationalUnitsForParent(input)
}

func (c *AwsClient) DescribeOrganizationalUnit(input *organizations.DescribeOrganizationalUnitInput) (*organizations.DescribeOrganizationalUnitOutput, error) {
	return c.orgClient.DescribeOrganizationalUnit(input)
}

func (c *AwsClient) TagResource(input *organizations.TagResourceInput) (*organizations.TagResourceOutput, error) {
	return c.orgClient.TagResource(input)
}

func (c *AwsClient) UntagResource(input *organizations.UntagResourceInput) (*organizations.UntagResourceOutput, error) {
	return c.orgClient.UntagResource(input)
}

func (c *AwsClient) ListTagsForResource(input *organizations.ListTagsForResourceInput) (*organizations.ListTagsForResourceOutput, error) {
	return c.orgClient.ListTagsForResource(input)
}

func (c *AwsClient) MoveAccount(input *organizations.MoveAccountInput) (*organizations.MoveAccountOutput, error) {
	return c.orgClient.MoveAccount(input)
}

func (c *AwsClient) GetResources(input *resourcegroupstaggingapi.GetResourcesInput) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	return c.resClient.GetResources(input)
}

func (c *AwsClient) GetCostAndUsage(input *costexplorer.GetCostAndUsageInput) (*costexplorer.GetCostAndUsageOutput, error) {
	return c.ceClient.GetCostAndUsage(input)
}

func (c *AwsClient) CreateCostCategoryDefinition(input *costexplorer.CreateCostCategoryDefinitionInput) (*costexplorer.CreateCostCategoryDefinitionOutput, error) {
	return c.ceClient.CreateCostCategoryDefinition(input)
}

func (c *AwsClient) ListCostCategoryDefinitions(input *costexplorer.ListCostCategoryDefinitionsInput) (*costexplorer.ListCostCategoryDefinitionsOutput, error) {
	return c.ceClient.ListCostCategoryDefinitions(input)
}

func (c *AwsClient) DescribeInstances(input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	return c.ec2Client.DescribeInstances(input)
}
