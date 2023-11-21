package osdCloud

import (
	"fmt"
	"strings"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	stsTypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	bpcloud "github.com/openshift/backplane-cli/cmd/ocm-backplane/cloud"
	bpconfig "github.com/openshift/backplane-cli/pkg/cli/config"
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

// GenerateOrganizationAccountAccessCredentials Uses the provided IAM Client to try and assume OrganizationAccountAccessRole for the given AWS Account
// This only works when the provided client is a user from the root account of an organization and the AWS account provided is a linked accounts within that organization
func GenerateOrganizationAccountAccessCredentials(client aws.Client, accountId, sessionName, partition string) (*stsTypes.Credentials, error) {

	roleArnString := aws.GenerateRoleARN(accountId, "OrganizationAccountAccessRole")

	targetRoleArn, err := arn.Parse(roleArnString)
	if err != nil {
		return nil, err
	}

	targetRoleArn.Partition = partition

	assumeRoleOutput, err := client.AssumeRole(
		&sts.AssumeRoleInput{
			RoleArn:         awsSdk.String(targetRoleArn.String()),
			RoleSessionName: &sessionName,
		},
	)
	if err != nil {
		return nil, err
	}

	return assumeRoleOutput.Credentials, nil

}

// GenerateSupportRoleCredentials Uses the provided IAM Client to perform the Assume Role chain needed to get to a cluster's Support Role
func GenerateSupportRoleCredentials(client aws.Client, region, sessionName, targetRole string) (*stsTypes.Credentials, error) {

	// Perform the Assume Role chain to get the jump
	jumpRoleCreds, err := GenerateJumpRoleCredentials(client, region, sessionName)
	if err != nil {
		return nil, err
	}

	// Build client for jump role needed for the last step
	jumpRoleClient, err := aws.NewAwsClientWithInput(
		&aws.ClientInput{
			AccessKeyID:     *jumpRoleCreds.AccessKeyId,
			SecretAccessKey: *jumpRoleCreds.SecretAccessKey,
			SessionToken:    *jumpRoleCreds.SessionToken,
			Region:          region,
		},
	)
	if err != nil {
		return nil, err
	}

	// Assume target ManagedOpenShift-Support role in the cluster's AWS Account
	targetAssumeRoleOutput, err := jumpRoleClient.AssumeRole(
		&sts.AssumeRoleInput{
			RoleArn:         &targetRole,
			RoleSessionName: &sessionName,
		},
	)
	if err != nil {
		return nil, err
	}

	return targetAssumeRoleOutput.Credentials, nil
}

// GenerateJumpRoleCredentials performs the Assume Role chain from IAM User to the Jump role
// This sequence stays within the Red Hat account boundary, so a failure here indicates an internal misconfiguration
func GenerateJumpRoleCredentials(client aws.Client, region, sessionName string) (*stsTypes.Credentials, error) {

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
			RoleArn:         &sreCcsAccessRoleArn,
			RoleSessionName: &sessionName,
		},
	)
	if err != nil {
		return nil, err
	}

	// Build client for RH-SRE-CCS-Access role
	sreCcsAccessRoleClient, err := aws.NewAwsClientWithInput(
		&aws.ClientInput{
			AccessKeyID:     *sreCcsAccessAssumeRoleOutput.Credentials.AccessKeyId,
			SecretAccessKey: *sreCcsAccessAssumeRoleOutput.Credentials.SecretAccessKey,
			SessionToken:    *sreCcsAccessAssumeRoleOutput.Credentials.SessionToken,
			Region:          region,
		},
	)
	if err != nil {
		return nil, err
	}

	jumpRoleKey := ProdJumproleConfigKey
	conn, err := utils.CreateConnection()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	currentEnv := utils.GetCurrentOCMEnv(conn)
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
			RoleArn:         &jumpRoleArn,
			RoleSessionName: &sessionName,
		},
	)
	if err != nil {
		return nil, err
	}

	return jumpAssumeRoleOutput.Credentials, nil

}

// GenerateRoleSessionName Uses the current IAM ARN to generate a role name. This should end up being RH-SRE-$kerberosID
func GenerateRoleSessionName(client aws.Client) (string, error) {

	callerIdentityOutput, err := client.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}

	roleArn, err := arn.Parse(*callerIdentityOutput.Arn)
	if err != nil {
		return "", err
	}

	splitArn := strings.Split(roleArn.Resource, "/")
	username := splitArn[1]

	return fmt.Sprintf("RH-SRE-%s", username), nil
}

// CreateAWSV2Config creates an aws-sdk-go-v2 config via Backplane given an internal cluster id
func CreateAWSV2Config(cluster *cmv1.Cluster) (awsSdk.Config, error) {
	bp, err := bpconfig.GetBackplaneConfiguration()
	if err != nil {
		return awsSdk.Config{}, fmt.Errorf("failed to load backplane-cli config: %v", err)
	}

	return bpcloud.GetAWSV2Config(bp.URL, cluster)
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
	assumedRoleCreds, err := GenerateSupportRoleCredentials(awsClient, clusterRegion, sessionName, targetRoleArn.String())
	if err != nil {
		return nil, err
	}

	awsClientCCS, err := aws.NewAwsClientWithInput(&aws.ClientInput{
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

	awsClientNonCCS, err := aws.NewAwsClientWithInput(&aws.ClientInput{
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
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return nil, err
	}
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

// AwsCluster Concrete struct with fields required only for interacting with the AWS cloud.
type AwsCluster struct {
	*BaseClient
	AZs        []string
	AwsProfile string
	AwsClient  aws.Client
}

func NewAwsCluster(ocmClient *sdk.Connection, clusterId string, awsProfile string) (ClusterHealthClient, error) {
	clusterResp, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(clusterId).Get().Send()
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	cluster := clusterResp.Body()
	return &AwsCluster{
		BaseClient: &BaseClient{
			ClusterId: clusterId,
			OcmClient: ocmClient,
			Cluster:   cluster,
		},
		AwsProfile: awsProfile,
	}, nil
}

func (a *AwsCluster) Login() error {
	awsClient, err := GenerateAWSClientForCluster(a.AwsProfile, a.ClusterId)
	a.AwsClient = awsClient
	a.AZs = a.Cluster.Nodes().AvailabilityZones()
	if err != nil {
		return err
	}
	return nil
}

func (a *AwsCluster) Close() {
}

func (a *AwsCluster) GetAZs() []string {
	return a.AZs
}

func (a *AwsCluster) GetAllVirtualMachines(string) ([]VirtualMachine, error) {
	vms := make([]VirtualMachine, 5)
	var nextToken *string
	for {
		instances, err := a.AwsClient.DescribeInstances(&ec2.DescribeInstancesInput{
			MaxResults: awsSdk.Int32(5),
			NextToken:  nextToken,
		})
		if err != nil {
			return nil, err
		}
		for idx := range instances.Reservations {
			for _, instance := range instances.Reservations[idx].Instances {
				stringTags := make(map[string]string, 0)
				var name string
				size := instance.InstanceType
				state := instance.State.Name
				for _, t := range instance.Tags {
					stringTags[*t.Key] = *t.Value
					if *t.Key == "Name" {
						name = *t.Value
					}
				}
				vm := VirtualMachine{
					Original: instance,
					Name:     name,
					Size:     string(size),
					State:    string(state),
					Labels:   stringTags,
				}
				vms = append(vms, vm)
			}
		}
		if instances.NextToken == nil {
			break
		}
		nextToken = instances.NextToken
	}
	return vms, nil
}
