package aws

// Generate client mocks for testing
//go:generate mockgen -source=client.go -package=mock -destination=mock/client.go

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/viper"
)

// ClientInput input for new aws client
type ClientInput struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
}

const (
	ProxyConfigKey = "aws_proxy"
	NoProxyFlag    = "skip-aws-proxy-check"
)

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
	DescribeRouteTables(*ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error)
	DescribeSubnets(*ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)
	DescribeVpcs(*ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)

	// Service Quotas
	ListServiceQuotas(*servicequotas.ListServiceQuotasInput) (*servicequotas.ListServiceQuotasOutput, error)
	RequestServiceQuotaIncrease(*servicequotas.RequestServiceQuotaIncreaseInput) (*servicequotas.RequestServiceQuotaIncreaseOutput, error)

	// Organizations
	CreateAccount(input *organizations.CreateAccountInput) (*organizations.CreateAccountOutput, error)
	DescribeCreateAccountStatus(input *organizations.DescribeCreateAccountStatusInput) (*organizations.DescribeCreateAccountStatusOutput, error)
	ListAccounts(input *organizations.ListAccountsInput) (*organizations.ListAccountsOutput, error)
	ListParents(input *organizations.ListParentsInput) (*organizations.ListParentsOutput, error)
	ListChildren(input *organizations.ListChildrenInput) (*organizations.ListChildrenOutput, error)
	ListRoots(input *organizations.ListRootsInput) (*organizations.ListRootsOutput, error)
	ListAccountsForParent(input *organizations.ListAccountsForParentInput) (*organizations.ListAccountsForParentOutput, error)
	ListOrganizationalUnitsForParent(input *organizations.ListOrganizationalUnitsForParentInput) (*organizations.ListOrganizationalUnitsForParentOutput, error)
	DescribeOrganizationalUnit(input *organizations.DescribeOrganizationalUnitInput) (*organizations.DescribeOrganizationalUnitOutput, error)
	TagResource(input *organizations.TagResourceInput) (*organizations.TagResourceOutput, error)
	UntagResource(input *organizations.UntagResourceInput) (*organizations.UntagResourceOutput, error)
	ListTagsForResource(input *organizations.ListTagsForResourceInput) (*organizations.ListTagsForResourceOutput, error)
	MoveAccount(input *organizations.MoveAccountInput) (*organizations.MoveAccountOutput, error)
	DescribeAccount(input *organizations.DescribeAccountInput) (*organizations.DescribeAccountOutput, error)

	// Resources
	GetResources(input *resourcegroupstaggingapi.GetResourcesInput) (*resourcegroupstaggingapi.GetResourcesOutput, error)

	// Cost Explorer
	GetCostAndUsage(input *costexplorer.GetCostAndUsageInput) (*costexplorer.GetCostAndUsageOutput, error)
	CreateCostCategoryDefinition(input *costexplorer.CreateCostCategoryDefinitionInput) (*costexplorer.CreateCostCategoryDefinitionOutput, error)
	ListCostCategoryDefinitions(input *costexplorer.ListCostCategoryDefinitionsInput) (*costexplorer.ListCostCategoryDefinitionsOutput, error)

	// Cloudtrail
	LookupEvents(input *cloudtrail.LookupEventsInput) (*cloudtrail.LookupEventsOutput, error)
}

type AwsClient struct {
	iamClient           iam.Client
	ec2Client           ec2.Client
	stsClient           sts.Client
	s3Client            s3.Client
	servicequotasClient servicequotas.Client
	orgClient           organizations.Client
	resClient           resourcegroupstaggingapi.Client
	ceClient            costexplorer.Client
	cloudTrailClient    cloudtrail.Client
}

func addProxyConfigToSessionOptConfig(config *aws.Config) {
	if viper.GetBool(NoProxyFlag) {
		fmt.Printf("Not adding proxy to AWS client due to presence of %v flag\n", NoProxyFlag)
		return
	}

	awsProxyUrl := viper.GetString(ProxyConfigKey)
	if awsProxyUrl == "" {
		_, _ = fmt.Fprintf(os.Stderr, "[ERROR] `%s` not configured. Please add this to your osdctl configuration to ensure traffic is routed though a proxy.\n", ProxyConfigKey)
		_, _ = fmt.Fprint(os.Stderr, "Please confirm that you would like to continue with [y|N] ")
		var input string
		_, _ = fmt.Scanln(&input)
		if strings.ToLower(input) != "y" {
			_, _ = fmt.Fprintln(os.Stderr, "Must enter 'y' to continue; exiting...")
			os.Exit(0)
		}
	} else {
		config.HTTPClient = &http.Client{
			Transport: &http.Transport{
				Proxy: func(*http.Request) (*url.URL, error) {
					return url.Parse(awsProxyUrl)
				},
			},
		}
	}
}

func NewAwsConfig(profile, region, configFile string) (*aws.Config, error) {
	var cfg aws.Config
	var err error

	// only set config file if it is not empty
	if configFile != "" {
		absCfgPath, err := filepath.Abs(configFile)
		if err != nil {
			return nil, fmt.Errorf("could not load config file: %w", err)
		}
		cfg, err = config.LoadDefaultConfig(context.TODO(), config.WithRegion(region), config.WithSharedConfigProfile(profile), config.WithSharedConfigFiles([]string{absCfgPath}))
	} else {
		cfg, err = config.LoadDefaultConfig(context.TODO(), config.WithRegion(region), config.WithSharedConfigProfile(profile))
		if err != nil {
			return nil, fmt.Errorf("error loading aws config: %w", err)
		}
	}

	addProxyConfigToSessionOptConfig(&cfg)

	if _, err := cfg.Credentials.Retrieve(context.TODO()); err != nil {
		return nil, fmt.Errorf("failed to retrieve AWS credentials: %w", err)
	}

	return &cfg, nil
}

// NewAwsClient creates an AWS client with credentials in the environment
func NewAwsClient(profile, region, configFile string) (Client, error) {
	cfg, err := NewAwsConfig(profile, region, configFile)
	if err != nil {
		return nil, err
	}

	awsClient := &AwsClient{
		iamClient:           *iam.NewFromConfig(*cfg),
		ec2Client:           *ec2.NewFromConfig(*cfg),
		stsClient:           *sts.NewFromConfig(*cfg),
		s3Client:            *s3.NewFromConfig(*cfg),
		servicequotasClient: *servicequotas.NewFromConfig(*cfg),
		orgClient:           *organizations.NewFromConfig(*cfg),
		ceClient:            *costexplorer.NewFromConfig(*cfg),
		resClient:           *resourcegroupstaggingapi.NewFromConfig(*cfg),
		cloudTrailClient:    *cloudtrail.NewFromConfig(*cfg),
	}

	// Validate the creds
	if _, err := awsClient.GetCallerIdentity(nil); err != nil {
		return nil, fmt.Errorf("error getting caller identity: %w", err)
	}

	return awsClient, nil
}

// NewAwsClientWithInput creates an AWS client with input credentials
func NewAwsClientWithInput(input *ClientInput) (Client, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(input.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(input.AccessKeyID, input.SecretAccessKey, input.SessionToken)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	addProxyConfigToSessionOptConfig(&cfg)

	return &AwsClient{
		iamClient:           *iam.NewFromConfig(cfg),
		ec2Client:           *ec2.NewFromConfig(cfg),
		stsClient:           *sts.NewFromConfig(cfg),
		s3Client:            *s3.NewFromConfig(cfg),
		servicequotasClient: *servicequotas.NewFromConfig(cfg),
		orgClient:           *organizations.NewFromConfig(cfg),
		ceClient:            *costexplorer.NewFromConfig(cfg),
		resClient:           *resourcegroupstaggingapi.NewFromConfig(cfg),
		cloudTrailClient:    *cloudtrail.NewFromConfig(cfg),
	}, nil
}

func (c *AwsClient) AssumeRole(input *sts.AssumeRoleInput) (*sts.AssumeRoleOutput, error) {
	return c.stsClient.AssumeRole(context.TODO(), input)
}

func (c *AwsClient) GetCallerIdentity(input *sts.GetCallerIdentityInput) (*sts.GetCallerIdentityOutput, error) {
	return c.stsClient.GetCallerIdentity(context.TODO(), input)
}

func (c *AwsClient) GetFederationToken(input *sts.GetFederationTokenInput) (*sts.GetFederationTokenOutput, error) {
	return c.stsClient.GetFederationToken(context.TODO(), input)
}

func (c *AwsClient) ListBuckets(input *s3.ListBucketsInput) (*s3.ListBucketsOutput, error) {
	return c.s3Client.ListBuckets(context.TODO(), input)
}

func (c *AwsClient) DeleteBucket(input *s3.DeleteBucketInput) (*s3.DeleteBucketOutput, error) {
	return c.s3Client.DeleteBucket(context.TODO(), input)
}

func (c *AwsClient) ListObjects(input *s3.ListObjectsInput) (*s3.ListObjectsOutput, error) {
	return c.s3Client.ListObjects(context.TODO(), input)
}

func (c *AwsClient) DeleteObjects(input *s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error) {
	return c.s3Client.DeleteObjects(context.TODO(), input)
}

func (c *AwsClient) CreateAccessKey(input *iam.CreateAccessKeyInput) (*iam.CreateAccessKeyOutput, error) {
	return c.iamClient.CreateAccessKey(context.TODO(), input)
}

func (c *AwsClient) DeleteAccessKey(input *iam.DeleteAccessKeyInput) (*iam.DeleteAccessKeyOutput, error) {
	return c.iamClient.DeleteAccessKey(context.TODO(), input)
}

func (c *AwsClient) ListAccessKeys(input *iam.ListAccessKeysInput) (*iam.ListAccessKeysOutput, error) {
	return c.iamClient.ListAccessKeys(context.TODO(), input)
}

func (c *AwsClient) GetUser(input *iam.GetUserInput) (*iam.GetUserOutput, error) {
	return c.iamClient.GetUser(context.TODO(), input)
}

func (c *AwsClient) CreateUser(input *iam.CreateUserInput) (*iam.CreateUserOutput, error) {
	return c.iamClient.CreateUser(context.TODO(), input)
}

func (c *AwsClient) ListUsers(input *iam.ListUsersInput) (*iam.ListUsersOutput, error) {
	return c.iamClient.ListUsers(context.TODO(), input)
}

func (c *AwsClient) ListPolicies(input *iam.ListPoliciesInput) (*iam.ListPoliciesOutput, error) {
	return c.iamClient.ListPolicies(context.TODO(), input)
}

func (c *AwsClient) AttachUserPolicy(input *iam.AttachUserPolicyInput) (*iam.AttachUserPolicyOutput, error) {
	return c.iamClient.AttachUserPolicy(context.TODO(), input)
}

func (c *AwsClient) CreatePolicy(input *iam.CreatePolicyInput) (*iam.CreatePolicyOutput, error) {
	return c.iamClient.CreatePolicy(context.TODO(), input)
}

func (c *AwsClient) DeletePolicy(input *iam.DeletePolicyInput) (*iam.DeletePolicyOutput, error) {
	return c.iamClient.DeletePolicy(context.TODO(), input)
}

func (c *AwsClient) AttachRolePolicy(input *iam.AttachRolePolicyInput) (*iam.AttachRolePolicyOutput, error) {
	return c.iamClient.AttachRolePolicy(context.TODO(), input)
}

func (c *AwsClient) DetachRolePolicy(input *iam.DetachRolePolicyInput) (*iam.DetachRolePolicyOutput, error) {
	return c.iamClient.DetachRolePolicy(context.TODO(), input)
}

func (c *AwsClient) ListAttachedRolePolicies(input *iam.ListAttachedRolePoliciesInput) (*iam.ListAttachedRolePoliciesOutput, error) {
	return c.iamClient.ListAttachedRolePolicies(context.TODO(), input)
}

func (c *AwsClient) DeleteLoginProfile(input *iam.DeleteLoginProfileInput) (*iam.DeleteLoginProfileOutput, error) {
	return c.iamClient.DeleteLoginProfile(context.TODO(), input)
}

func (c *AwsClient) ListSigningCertificates(input *iam.ListSigningCertificatesInput) (*iam.ListSigningCertificatesOutput, error) {
	return c.iamClient.ListSigningCertificates(context.TODO(), input)
}

func (c *AwsClient) DeleteSigningCertificate(input *iam.DeleteSigningCertificateInput) (*iam.DeleteSigningCertificateOutput, error) {
	return c.iamClient.DeleteSigningCertificate(context.TODO(), input)
}

func (c *AwsClient) ListUserPolicies(input *iam.ListUserPoliciesInput) (*iam.ListUserPoliciesOutput, error) {
	return c.iamClient.ListUserPolicies(context.TODO(), input)
}

func (c *AwsClient) DeleteUserPolicy(input *iam.DeleteUserPolicyInput) (*iam.DeleteUserPolicyOutput, error) {
	return c.iamClient.DeleteUserPolicy(context.TODO(), input)
}

func (c *AwsClient) ListAttachedUserPolicies(input *iam.ListAttachedUserPoliciesInput) (*iam.ListAttachedUserPoliciesOutput, error) {
	return c.iamClient.ListAttachedUserPolicies(context.TODO(), input)
}

func (c *AwsClient) DetachUserPolicy(input *iam.DetachUserPolicyInput) (*iam.DetachUserPolicyOutput, error) {
	return c.iamClient.DetachUserPolicy(context.TODO(), input)
}

func (c *AwsClient) ListGroupsForUser(input *iam.ListGroupsForUserInput) (*iam.ListGroupsForUserOutput, error) {
	return c.iamClient.ListGroupsForUser(context.TODO(), input)
}

func (c *AwsClient) RemoveUserFromGroup(input *iam.RemoveUserFromGroupInput) (*iam.RemoveUserFromGroupOutput, error) {
	return c.iamClient.RemoveUserFromGroup(context.TODO(), input)
}

func (c *AwsClient) ListRoles(input *iam.ListRolesInput) (*iam.ListRolesOutput, error) {
	return c.iamClient.ListRoles(context.TODO(), input)
}

func (c *AwsClient) DeleteRole(input *iam.DeleteRoleInput) (*iam.DeleteRoleOutput, error) {
	return c.iamClient.DeleteRole(context.TODO(), input)
}

func (c *AwsClient) DeleteUser(input *iam.DeleteUserInput) (*iam.DeleteUserOutput, error) {
	return c.iamClient.DeleteUser(context.TODO(), input)
}

func (c *AwsClient) ListAccounts(input *organizations.ListAccountsInput) (*organizations.ListAccountsOutput, error) {
	return c.orgClient.ListAccounts(context.TODO(), input)
}

func (c *AwsClient) ListParents(input *organizations.ListParentsInput) (*organizations.ListParentsOutput, error) {
	return c.orgClient.ListParents(context.TODO(), input)
}

func (c *AwsClient) ListChildren(input *organizations.ListChildrenInput) (*organizations.ListChildrenOutput, error) {
	return c.orgClient.ListChildren(context.TODO(), input)
}
func (c *AwsClient) ListRoots(input *organizations.ListRootsInput) (*organizations.ListRootsOutput, error) {
	return c.orgClient.ListRoots(context.TODO(), input)
}
func (c *AwsClient) ListAccountsForParent(input *organizations.ListAccountsForParentInput) (*organizations.ListAccountsForParentOutput, error) {
	return c.orgClient.ListAccountsForParent(context.TODO(), input)
}

func (c *AwsClient) ListServiceQuotas(input *servicequotas.ListServiceQuotasInput) (*servicequotas.ListServiceQuotasOutput, error) {
	return c.servicequotasClient.ListServiceQuotas(context.TODO(), input)
}

func (c *AwsClient) RequestServiceQuotaIncrease(input *servicequotas.RequestServiceQuotaIncreaseInput) (*servicequotas.RequestServiceQuotaIncreaseOutput, error) {
	return c.servicequotasClient.RequestServiceQuotaIncrease(context.TODO(), input)
}

func (c *AwsClient) CreateAccount(input *organizations.CreateAccountInput) (*organizations.CreateAccountOutput, error) {
	return c.orgClient.CreateAccount(context.TODO(), input)
}

func (c *AwsClient) DescribeCreateAccountStatus(input *organizations.DescribeCreateAccountStatusInput) (*organizations.DescribeCreateAccountStatusOutput, error) {
	return c.orgClient.DescribeCreateAccountStatus(context.TODO(), input)
}

func (c *AwsClient) ListOrganizationalUnitsForParent(input *organizations.ListOrganizationalUnitsForParentInput) (*organizations.ListOrganizationalUnitsForParentOutput, error) {
	return c.orgClient.ListOrganizationalUnitsForParent(context.TODO(), input)
}

func (c *AwsClient) DescribeOrganizationalUnit(input *organizations.DescribeOrganizationalUnitInput) (*organizations.DescribeOrganizationalUnitOutput, error) {
	return c.orgClient.DescribeOrganizationalUnit(context.TODO(), input)
}

func (c *AwsClient) TagResource(input *organizations.TagResourceInput) (*organizations.TagResourceOutput, error) {
	return c.orgClient.TagResource(context.TODO(), input)
}

func (c *AwsClient) UntagResource(input *organizations.UntagResourceInput) (*organizations.UntagResourceOutput, error) {
	return c.orgClient.UntagResource(context.TODO(), input)
}

func (c *AwsClient) ListTagsForResource(input *organizations.ListTagsForResourceInput) (*organizations.ListTagsForResourceOutput, error) {
	return c.orgClient.ListTagsForResource(context.TODO(), input)
}

func (c *AwsClient) MoveAccount(input *organizations.MoveAccountInput) (*organizations.MoveAccountOutput, error) {
	return c.orgClient.MoveAccount(context.TODO(), input)
}

func (c *AwsClient) DescribeAccount(input *organizations.DescribeAccountInput) (*organizations.DescribeAccountOutput, error) {
	return c.orgClient.DescribeAccount(context.TODO(), input)
}

func (c *AwsClient) GetResources(input *resourcegroupstaggingapi.GetResourcesInput) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	return c.resClient.GetResources(context.TODO(), input)
}

func (c *AwsClient) GetCostAndUsage(input *costexplorer.GetCostAndUsageInput) (*costexplorer.GetCostAndUsageOutput, error) {
	return c.ceClient.GetCostAndUsage(context.TODO(), input)
}

func (c *AwsClient) CreateCostCategoryDefinition(input *costexplorer.CreateCostCategoryDefinitionInput) (*costexplorer.CreateCostCategoryDefinitionOutput, error) {
	return c.ceClient.CreateCostCategoryDefinition(context.TODO(), input)
}

func (c *AwsClient) ListCostCategoryDefinitions(input *costexplorer.ListCostCategoryDefinitionsInput) (*costexplorer.ListCostCategoryDefinitionsOutput, error) {
	return c.ceClient.ListCostCategoryDefinitions(context.TODO(), input)
}

func (c *AwsClient) DescribeInstances(input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	return c.ec2Client.DescribeInstances(context.TODO(), input)
}

func (c *AwsClient) DescribeRouteTables(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
	return c.ec2Client.DescribeRouteTables(context.TODO(), input)
}

func (c *AwsClient) DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	return c.ec2Client.DescribeSubnets(context.TODO(), input)
}

func (c *AwsClient) DescribeVpcs(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	return c.ec2Client.DescribeVpcs(context.TODO(), input)
}

func (c *AwsClient) StopInstances(input *ec2.StopInstancesInput) (*ec2.StopInstancesOutput, error) {
	return c.ec2Client.StopInstances(context.TODO(), input)
}

func (c *AwsClient) ModifyInstanceAttribute(input *ec2.ModifyInstanceAttributeInput) (*ec2.ModifyInstanceAttributeOutput, error) {
	return c.ec2Client.ModifyInstanceAttribute(context.TODO(), input)
}

func (c *AwsClient) StartInstances(input *ec2.StartInstancesInput) (*ec2.StartInstancesOutput, error) {
	return c.ec2Client.StartInstances(context.TODO(), input)
}

func (c *AwsClient) LookupEvents(input *cloudtrail.LookupEventsInput) (*cloudtrail.LookupEventsOutput, error) {
	return c.cloudTrailClient.LookupEvents(context.TODO(), input)
}
