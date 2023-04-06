package network

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift-online/ocm-sdk-go/logging"
	"github.com/openshift/osd-network-verifier/pkg/proxy"
	onv "github.com/openshift/osd-network-verifier/pkg/verifier"
	onvAwsClient "github.com/openshift/osd-network-verifier/pkg/verifier/aws"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

const nonByovpcPrivateSubnetTagKey = "kubernetes.io/role/internal-elb"

type EgressVerification struct {
	awsClient egressVerificationAWSClient
	cluster   *cmv1.Cluster
	log       logging.Logger

	// ClusterId is the internal or external OCM cluster ID.
	// This is optional, but typically is used to automatically detect the correct settings.
	ClusterId string
	// AWS Region is an optional override if not specified via AWS credentials.
	Region string
	// SubnetId is an optional override for specifying an AWS subnet ID.
	// Must be a private subnet to provide accurate results.
	SubnetId []string
	// SecurityGroupId is an optional override for specifying an AWS security group ID.
	SecurityGroupId string
	// Debug optionally enables debug-level logging for underlying calls to osd-network-verifier.
	Debug bool
	// CaCert is an optional path to an additional Certificate Authority (CA) trust bundle, which is commonly used by a
	// cluster-wide proxy.
	// Docs: https://docs.openshift.com/rosa/networking/configuring-cluster-wide-proxy.html
	CaCert string
	// NoTls optionally ignores SSL validation on the client-side.
	NoTls bool
	// AllSubnets is an option for multi-AZ clusters that will run the network verification against all subnets listed by ocm
	AllSubnets bool
}

func NewCmdValidateEgress() *cobra.Command {
	e := &EgressVerification{}

	validateEgressCmd := &cobra.Command{
		Use:   "verify-egress",
		Short: "Verify an AWS OSD/ROSA cluster can reach all required external URLs necessary for full support.",
		Long: `Verify an AWS OSD/ROSA cluster can reach all required external URLs necessary for full support.

  This command is an opinionated wrapper around running https://github.com/openshift/osd-network-verifier for SREs.
  Given an OCM cluster name or id, this command will attempt to automatically detect the security group, subnet, and
  cluster-wide proxy configuration to run osd-network-verifier's egress verification. The purpose of this check is to
  verify whether a ROSA cluster's VPC allows for all required external URLs are reachable. The exact cause can vary and
  typically requires a customer to remediate the issue themselves.

  Docs: https://docs.openshift.com/rosa/rosa_install_access_delete_clusters/rosa_getting_started_iam/rosa-aws-prereqs.html#osd-aws-privatelink-firewall-prerequisites_prerequisites`,
		Example: `
  # Run against a cluster registered in OCM
  ocm-backplane tunnel -D
  osdctl network verify-egress --cluster-id my-rosa-cluster

  # Run against a cluster registered in OCM with a cluster-wide-proxy
  ocm-backplane tunnel -D
  touch cacert.txt
  osdctl network verify-egress --cluster-id my-rosa-cluster --cacert cacert.txt

  # Override automatic selection of a subnet or security group id
  ocm-backplane tunnel -D
  osdctl network verify-egress --cluster-id my-rosa-cluster --subnet-id subnet-abcd --security-group sg-abcd

  # (Not recommended) Run against a specific VPC, without specifying cluster-id
  <export environment variables like AWS_ACCESS_KEY_ID or use aws configure>
  osdctl network verify-egress --subnet-id subnet-abcdefg123 --security-group sg-abcdefgh123 --region us-east-1`,
		Run: func(cmd *cobra.Command, args []string) {
			e.Run(context.TODO())
		},
	}

	validateEgressCmd.Flags().StringVarP(&e.ClusterId, "cluster-id", "C", "", "(optional) OCM internal/external cluster id to run osd-network-verifier against.")
	validateEgressCmd.Flags().StringArrayVar(&e.SubnetId, "subnet-id", nil, "(optional) private subnet ID override, required if not specifying --cluster-id and can be specified multiple times to run against multiple subnets")
	validateEgressCmd.Flags().StringVar(&e.SecurityGroupId, "security-group", "", "(optional) security group ID override for osd-network-verifier, required if not specifying --cluster-id")
	validateEgressCmd.Flags().StringVar(&e.CaCert, "cacert", "", "(optional) path to a file containing the additional CA trust bundle. Typically set so that the verifier can use a configured cluster-wide proxy.")
	validateEgressCmd.Flags().BoolVar(&e.NoTls, "no-tls", false, "(optional) if provided, ignore all ssl certificate validations on client-side.")
	validateEgressCmd.Flags().StringVar(&e.Region, "region", "", "(optional) AWS region")
	validateEgressCmd.Flags().BoolVar(&e.Debug, "debug", false, "(optional) if provided, enable additional debug-level logging")
	validateEgressCmd.Flags().BoolVarP(&e.AllSubnets, "all-subnets", "A", false, "(optional) an option for Privatelink clusters to run osd-network-verifier against all subnets listed by ocm.")

	// If a cluster-id is specified, don't allow the foot-gun of overriding region
	validateEgressCmd.MarkFlagsMutuallyExclusive("cluster-id", "region")

	return validateEgressCmd
}

type egressVerificationAWSClient interface {
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
}

// Run parses the EgressVerification input, typically sets values automatically using the ClusterId, and runs
// osd-network-verifier's egress check to validate AWS firewall prerequisites for ROSA.
// Docs: https://docs.openshift.com/rosa/rosa_install_access_delete_clusters/rosa_getting_started_iam/rosa-aws-prereqs.html#osd-aws-privatelink-firewall-prerequisites_prerequisites
func (e *EgressVerification) Run(ctx context.Context) {
	cfg, err := e.setup(ctx)
	if err != nil {
		log.Fatal(err)
	}

	c, err := onvAwsClient.NewAwsVerifierFromConfig(*cfg, e.log)
	if err != nil {
		log.Fatal(fmt.Errorf("failed to assemble osd-network-verifier client: %s", err))
	}

	inputs, err := e.generateAWSValidateEgressInput(ctx, cfg.Region)
	if err != nil {
		log.Fatal(err)
	}

	for i := range inputs {
		e.log.Info(ctx, "running with config: %+v", inputs[i])
		e.log.Info(ctx, "running network verifier for subnet  %+v, security group %+v", inputs[i].SubnetID, inputs[i].AWS.SecurityGroupId)
		out := onv.ValidateEgress(c, *inputs[i])
		out.Summary(e.Debug)
		if out.IsSuccessful() {
			log.Println("All tests pass")
		}
	}
}

// setup configures an EgressVerification's awsClient and cluster depending on whether the ClusterId or profile
// flags are supplied. It also returns an aws.Config if needed.
func (e *EgressVerification) setup(ctx context.Context) (*aws.Config, error) {
	// Setup logger
	builder := logging.NewGoLoggerBuilder()
	if e.Debug {
		builder.Debug(true)
	}
	logger, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("network verification failed to build logger: %s", err)
	}
	e.log = logger

	// If ClusterId is supplied, leverage ocm and ocm-backplane to get an AWS client
	if e.ClusterId != "" {
		e.log.Debug(ctx, "searching OCM for cluster: %s", e.ClusterId)
		ocmClient := utils.CreateConnection()
		defer ocmClient.Close()

		cluster, err := utils.GetClusterAnyStatus(ocmClient, e.ClusterId)
		if err != nil {
			return nil, fmt.Errorf("failed to get OCM cluster info for %s: %s", e.ClusterId, err)
		}
		e.log.Debug(ctx, "cluster %s found from OCM: %s", e.ClusterId, cluster.ID())
		e.cluster = cluster

		e.log.Info(ctx, "getting AWS credentials from backplane-api")
		cfg, err := osdCloud.CreateAWSV2Config(ctx, cluster.ID())
		if err != nil {
			return nil, err
		}
		e.log.Debug(ctx, "retrieved AWS credentials from backplane-api")
		e.awsClient = ec2.NewFromConfig(cfg)
		return &cfg, nil
	}

	// If no ClusterId is supplied, then --subnet-id and --security-group are required
	if e.SubnetId == nil || e.SecurityGroupId == "" {
		return nil, fmt.Errorf("--subnet-id and --security-group are required when --cluster-id is not specified")
	}

	e.log.Info(ctx, "[WARNING] no cluster-id specified, there is reduced validation around the security group, subnet, and proxy, causing inaccurate results")
	e.log.Info(ctx, "using whatever default AWS credentials are locally available")
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("network verification failed to find valid creds locally: %s", err)
	}

	// Additionally, if an AWS region must be provided somehow if there's no ClusterId
	// This could have been done via the default AWS credentials or can be supplied manually via --region
	if e.Region != "" {
		e.log.Info(ctx, "overriding region with %s", e.Region)
		cfg.Region = e.Region
	}

	return &cfg, nil
}

// generateAWSValidateEgressInput is an opinionated interface in front of osd-network-verifier.
// Its input is an OCM internal/external ClusterId and it returns the corresponding input to osd-network-verifier with
// default AWS tags, one of the cluster's private subnet IDs, and the cluster's master security group.
// Can override SecurityGroupId and SubnetId.
func (e *EgressVerification) generateAWSValidateEgressInput(ctx context.Context, region string) ([]*onv.ValidateEgressInput, error) {
	// We can auto-detect information from OCM
	if e.cluster != nil {
		// TODO: osd-network-verifier technically does support GCP, but just handle AWS for now
		if e.cluster.CloudProvider().ID() != "aws" {
			return nil, fmt.Errorf("only supports aws, got %s", e.cluster.CloudProvider().ID())
		}

		if e.cluster.Product().ID() != "rosa" &&
			e.cluster.Product().ID() != "osd" &&
			e.cluster.Product().ID() != "osdtrial" {
			return nil, fmt.Errorf("only supports rosa, osd, and osdtrial, got %s", e.cluster.Product().ID())
		}
	}

	input, err := defaultValidateEgressInput(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble validate egress input: %s", err)
	}

	// Setup proxy configuration that is not automatically determined
	input.Proxy.NoTls = e.NoTls
	if e.CaCert != "" {
		cert, err := os.ReadFile(e.CaCert)
		if err != nil {
			return nil, err
		}
		input.Proxy.Cacert = bytes.NewBuffer(cert).String()
	}

	// If the cluster has a cluster-wide proxy, configure it
	if e.cluster != nil {
		if e.cluster.Proxy() != nil && !e.cluster.Proxy().Empty() {
			input.Proxy.HttpProxy = e.cluster.Proxy().HTTPProxy()
			input.Proxy.HttpsProxy = e.cluster.Proxy().HTTPSProxy()

			// The actual trust bundle is redacted in OCM, but is an indicator that --cacert is required
			if e.cluster.AdditionalTrustBundle() != "" && e.CaCert == "" {
				return nil, fmt.Errorf("%s has an additional trust bundle configured, but no --cacert supplied", e.ClusterId)
			}
		}
	}

	// Fill in subnetID
	subnetId, err := e.getSubnetId(context.TODO())
	if err != nil {
		return nil, err
	}
	input.SubnetID = subnetId[0]

	// Fill in securityGroupID
	sgId, err := e.getSecurityGroupId(context.TODO())
	if err != nil {
		return nil, err
	}
	input.AWS.SecurityGroupId = sgId

	//Creating a slice of input values to run in a for loop in the network-verifier
	inputs := make([]*onv.ValidateEgressInput, len(subnetId))
	for i := range subnetId {
		inputs[i] = input
		inputs[i].SubnetID = subnetId[i]

	}

	return inputs, nil
}

// getSubnetId attempts to return a private subnet ID.
// e.SubnetId acts as an override, otherwise e.awsClient will be used to attempt to determine the correct subnets
func (e *EgressVerification) getSubnetId(ctx context.Context) ([]string, error) {
	// A SubnetId was manually specified, just use that
	if e.SubnetId != nil {
		e.log.Info(ctx, "using manually specified subnet-id: %s", e.SubnetId)
		return e.SubnetId, nil
	}

	// If this is a non-BYOVPC cluster, we can find the private subnets based on the cluster and internal-elb tag
	if len(e.cluster.AWS().SubnetIDs()) == 0 {
		e.log.Info(ctx, "searching for subnets by tags: kubernetes.io/cluster/%s=owned and %s=", e.cluster.InfraID(), nonByovpcPrivateSubnetTagKey)
		resp, err := e.awsClient.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
			Filters: []types.Filter{
				{
					Name:   aws.String(fmt.Sprintf("tag:kubernetes.io/cluster/%s", e.cluster.InfraID())),
					Values: []string{"owned"},
				},
				{
					Name:   aws.String("tag-key"),
					Values: []string{nonByovpcPrivateSubnetTagKey},
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to find private subnets for %s: %w", e.cluster.InfraID(), err)
		}

		if len(resp.Subnets) == 0 {
			return nil, fmt.Errorf("found 0 subnets with kubernetes.io/cluster/%s=owned and %s, consider the --subnet-id flag", e.cluster.InfraID(), e.cluster.InfraID())
		}
		if e.AllSubnets {
			subnets := make([]string, len(resp.Subnets))
			for i := range resp.Subnets {
				subnets[i] = *resp.Subnets[i].SubnetId
			}
			return subnets, nil
		} else {
			e.log.Info(ctx, "using subnet-id: %s", *resp.Subnets[0].SubnetId)
			return []string{*resp.Subnets[0].SubnetId}, nil
		}
	}

	// For PrivateLink clusters, any provided subnet is considered a private subnet
	if e.cluster.AWS().PrivateLink() {
		if len(e.cluster.AWS().SubnetIDs()) == 0 {
			return nil, fmt.Errorf("unexpected error: %s is a PrivateLink cluster, but no subnets in OCM", e.cluster.InfraID())
		}
		// If the all-subnets flag is on, the network verifier will iterate over all subnets listed by ocm
		if e.AllSubnets {

			subnets := e.cluster.AWS().SubnetIDs()

			e.log.Debug(ctx, "Found the following subnets listed with ocm: %v", subnets)

			//add an error to handle
			e.log.Debug(ctx, "Assigned value to var e.SubnetId: %v", subnets)
			return subnets, nil

		}

		e.log.Info(ctx, "detected BYOVPC PrivateLink cluster, using first subnet from OCM: %s", e.cluster.AWS().SubnetIDs()[0])
		return []string{e.cluster.AWS().SubnetIDs()[0]}, nil

	}

	// For non-PrivateLink BYOVPC clusters, provided subnets are 50/50 public/private subnets, so make the user decide for now
	// TODO: Figure out via IGW/NAT GW/Route Tables
	return nil, fmt.Errorf("unable to determine which non-PrivateLink BYOVPC subnets are private yet, please check manually and provide the --subnet-id flag")
}

// getSecurityGroupId attempts to return a cluster's master node security group Id
// e.SecurityGroupId acts as an override, otherwise e.awsClient will be used to attempt to determine the correct security group
func (e *EgressVerification) getSecurityGroupId(ctx context.Context) (string, error) {
	// A SecurityGroupId was manually specified, just use that
	if e.SecurityGroupId != "" {
		e.log.Info(ctx, "using manually specified security-group-id: %s", e.SecurityGroupId)
		return e.SecurityGroupId, nil
	}

	// If no SecurityGroupId override is passed in, try to determine the master security group id
	e.log.Info(ctx, "searching for security group by tags: kubernetes.io/cluster/%s=owned and Name=%s-master-sg", e.cluster.InfraID(), e.cluster.InfraID())
	resp, err := e.awsClient.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{fmt.Sprintf("%s-master-sg", e.cluster.InfraID())},
			},
			{
				Name:   aws.String(fmt.Sprintf("tag:kubernetes.io/cluster/%s", e.cluster.InfraID())),
				Values: []string{"owned"},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to find master security group for %s: %w", e.cluster.InfraID(), err)
	}

	if len(resp.SecurityGroups) == 0 {
		return "", fmt.Errorf("failed to find any master security groups by tag: kubernetes.io/cluster/%s=owned and Name==%s-master-sg", e.cluster.InfraID(), e.cluster.InfraID())
	}

	e.log.Info(ctx, "using security-group-id: %s", *resp.SecurityGroups[0].GroupId)
	return *resp.SecurityGroups[0].GroupId, nil
}

// defaultValidateEgressInput generates an opinionated default osd-network-verifier ValidateEgressInput.
func defaultValidateEgressInput(ctx context.Context, region string) (*onv.ValidateEgressInput, error) {
	awsDefaultTags := map[string]string{
		"osd-network-verifier": "owned",
		"red-hat-managed":      "true",
		"Name":                 "osd-network-verifier",
	}

	if onvAwsClient.GetAMIForRegion(region) == "" {
		return nil, fmt.Errorf("unsupported region: %s", region)
	}

	return &onv.ValidateEgressInput{
		Timeout:      2 * time.Second,
		Ctx:          ctx,
		SubnetID:     "",
		CloudImageID: onvAwsClient.GetAMIForRegion(region),
		InstanceType: "t3.micro",
		Proxy:        proxy.ProxyConfig{},
		Tags:         awsDefaultTags,
		AWS: onv.AwsEgressConfig{
			SecurityGroupId: "",
		},
	}, nil
}
