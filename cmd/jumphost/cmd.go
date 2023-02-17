package jumphost

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

const (
	awsResourceName     = "red-hat-sre-jumphost"
	publicSubnetTagKey  = "kubernetes.io/role/elb"
	privateSubnetTagKey = "kubernetes.io/role/internal-elb"
)

func NewCmdJumphost() *cobra.Command {
	jumphost := &cobra.Command{
		Use:  "jumphost",
		Args: cobra.NoArgs,
	}

	jumphost.AddCommand(
		newCmdCreateJumphost(),
		newCmdDeleteJumphost(),
	)

	return jumphost
}

type jumphostConfig struct {
	awsClient jumphostAWSClient
	cluster   *cmv1.Cluster
	subnetId  string
	tags      []types.Tag

	keyFilepath string
	ec2PublicIp string
}

type jumphostAWSClient interface {
	CreateKeyPair(ctx context.Context, params *ec2.CreateKeyPairInput, optFns ...func(options *ec2.Options)) (*ec2.CreateKeyPairOutput, error)
	DeleteKeyPair(ctx context.Context, params *ec2.DeleteKeyPairInput, optFns ...func(options *ec2.Options)) (*ec2.DeleteKeyPairOutput, error)
	DescribeKeyPairs(ctx context.Context, params *ec2.DescribeKeyPairsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeKeyPairsOutput, error)

	AuthorizeSecurityGroupIngress(ctx context.Context, params *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(options *ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	CreateSecurityGroup(ctx context.Context, params *ec2.CreateSecurityGroupInput, optFns ...func(options *ec2.Options)) (*ec2.CreateSecurityGroupOutput, error)
	DeleteSecurityGroup(ctx context.Context, params *ec2.DeleteSecurityGroupInput, optFns ...func(options *ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error)
	DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)

	DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeImagesOutput, error)
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	RunInstances(ctx context.Context, params *ec2.RunInstancesInput, optFns ...func(options *ec2.Options)) (*ec2.RunInstancesOutput, error)
	TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(options *ec2.Options)) (*ec2.TerminateInstancesOutput, error)

	// CreateTags (ec2:CreateTags) is not used explicitly, but all AWS resources will be created with tags
	CreateTags(ctx context.Context, params *ec2.CreateTagsInput, optFns ...func(options *ec2.Options)) (*ec2.CreateTagsOutput, error)
}

// initJumphostConfig initializes a jumphostConfig struct for use with jumphost commands.
// Generally, this function should always be used as opposed to initializing the struct by hand.
func initJumphostConfig(ctx context.Context, clusterId, subnetId string) (*jumphostConfig, error) {
	ocm := utils.CreateConnection()
	defer ocm.Close()

	//cluster, err := utils.GetClusterAnyStatus(ocm, clusterId)
	//if err != nil {
	//	return nil, fmt.Errorf("failed to get OCM cluster info for %s: %s", clusterId, err)
	//}

	//if err := validateCluster(cluster); err != nil {
	//	return nil, fmt.Errorf("cluster not supported yet - %s", err)
	//}
	//
	//log.Printf("getting AWS credentials from backplane-api for %s (%s)", cluster.Name(), cluster.ID())
	//cfg, err := osdCloud.CreateAWSV2Config(ctx, cluster.ID())
	//if err != nil {
	//	return nil, err
	//}

	// TODO: When --cluster-id is supported, only do this as a fallback
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	return &jumphostConfig{
		awsClient: ec2.NewFromConfig(cfg),
		subnetId:  subnetId,
		tags: []types.Tag{
			//{
			//	// This tag will allow the uninstaller to clean up orphaned resources in worst-case scenarios
			//	Key:   aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", cluster.InfraID())),
			//	Value: aws.String("owned"),
			//},
			{
				Key:   aws.String("red-hat-managed"),
				Value: aws.String("true"),
			},
			{
				Key:   aws.String("Name"),
				Value: aws.String("red-hat-sre-jumphost"),
			},
		},
	}, nil
}

// validateCluster is currently unused as the --cluster-id flag is not supported yet.
// Eventually, it will gate the usage of the --cluster-id flag based on types of supported clusters.
func validateCluster(cluster *cmv1.Cluster) error {
	if cluster != nil {
		if cluster.CloudProvider().ID() != "aws" {
			return fmt.Errorf("only supports aws, got %s", cluster.CloudProvider().ID())
		}

		if !cluster.AWS().STS().Empty() {
			return errors.New("only supports non-STS clusters")
		}

		return nil
	}

	return errors.New("unexpected error, nil cluster provided")
}

// generateTagFilters converts a slice of expected tags to a slice of corresponding filters to search by.
func generateTagFilters(tags []types.Tag) []types.Filter {
	if len(tags) == 0 {
		return nil
	}

	filters := make([]types.Filter, len(tags))
	for i, tag := range tags {
		filters[i] = types.Filter{
			Name:   aws.String(fmt.Sprintf("tag:%s", *tag.Key)),
			Values: []string{*tag.Value},
		}
	}

	return filters
}
