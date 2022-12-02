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

type egressVerification struct {
	awsClient egressVerificationAWSClient
	cluster   *cmv1.Cluster
	log       logging.Logger

	// clusterId is the cluster identifier that will be used to query backplane for AWS credentials to build an AWS client
	clusterId       string
	region          string
	subnetId        string
	securityGroupId string
	debug           bool
	caCert          string
	noTls           bool
}

func NewCmdValidateEgress() *cobra.Command {
	e := &egressVerification{}

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
			e.run(context.TODO())
		},
	}

	validateEgressCmd.Flags().StringVar(&e.clusterId, "cluster-id", "", "(optional) OCM internal/external cluster id to run osd-network-verifier against.")
	validateEgressCmd.Flags().StringVar(&e.subnetId, "subnet-id", "", "(optional) private subnet ID override, required if not specifying --cluster-id")
	validateEgressCmd.Flags().StringVar(&e.securityGroupId, "security-group", "", "(optional) security group ID override for osd-network-verifier, required if not specifying --cluster-id")
	validateEgressCmd.Flags().StringVar(&e.caCert, "cacert", "", "(optional) path to cacert file to be used with https requests being made by verifier")
	validateEgressCmd.Flags().BoolVar(&e.noTls, "no-tls", false, "(optional) if provided, ignore all ssl certificate validations on client-side.")
	validateEgressCmd.Flags().StringVar(&e.region, "region", "", "(optional) AWS region")
	validateEgressCmd.Flags().BoolVar(&e.debug, "debug", false, "(optional) if provided, enable additional debug-level logging")

	// If a cluster-id is specified, don't allow the foot-gun of overriding region
	validateEgressCmd.MarkFlagsMutuallyExclusive("cluster-id", "region")

	return validateEgressCmd
}

type egressVerificationAWSClient interface {
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
}

// run parses the egressVerification input, typically sets values automatically using the clusterId, and runs
// osd-network-verifier's egress check to validate AWS firewall prerequisites for ROSA.
func (e *egressVerification) run(ctx context.Context) {
	cfg, err := e.setup(ctx)
	if err != nil {
		log.Fatal(err)
	}

	c, err := onvAwsClient.NewAwsVerifierFromConfig(*cfg, e.log)
	if err != nil {
		log.Fatal(fmt.Errorf("failed to assemble osd-network-verifier client: %s", err))
	}

	input, err := e.generateAWSValidateEgressInput(ctx, cfg.Region)
	if err != nil {
		log.Fatal(err)
	}
	e.log.Info(ctx, "running with config: %+v", input)

	out := onv.ValidateEgress(c, *input)
	out.Summary(e.debug)
	if out.IsSuccessful() {
		log.Println("All tests pass")
	}
}

// setup configures an egressVerification's awsClient and cluster depending on whether the clusterId or profile
// flags are supplied. It also returns an aws.Config if needed.
func (e *egressVerification) setup(ctx context.Context) (*aws.Config, error) {
	// Setup logger
	builder := logging.NewGoLoggerBuilder()
	if e.debug {
		builder.Debug(true)
	}
	logger, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("network verification failed to build logger: %s", err)
	}
	e.log = logger

	// If clusterId is supplied, leverage ocm and ocm-backplane to get an AWS client
	if e.clusterId != "" {
		e.log.Debug(ctx, "searching OCM for cluster: %s", e.clusterId)
		ocmClient := utils.CreateConnection()
		defer ocmClient.Close()

		cluster, err := utils.GetClusterAnyStatus(ocmClient, e.clusterId)
		if err != nil {
			return nil, fmt.Errorf("failed to get OCM cluster info for %s: %s", e.clusterId, err)
		}
		e.log.Debug(ctx, "cluster %s found from OCM: %s", e.clusterId, cluster.ID())
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

	// If no clusterId is supplied, then --subnet-id and --security-group are required
	if e.subnetId == "" || e.securityGroupId == "" {
		return nil, fmt.Errorf("--subnet-id and --security-group are required when --cluster-id is not specified")
	}

	e.log.Info(ctx, "[WARNING] no cluster-id specified, there is reduced validation around the security group, subnet, and proxy, causing inaccurate results")
	e.log.Info(ctx, "using whatever default AWS credentials are locally available")
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("network verification failed to find valid creds locally: %s", err)
	}

	// Additionally, if an AWS region must be provided somehow if there's no clusterId
	// This could have been done via the default AWS credentials or can be supplied manually via --region
	if e.region != "" {
		e.log.Info(ctx, "overriding region with %s", e.region)
		cfg.Region = e.region
	}

	return &cfg, nil
}

// generateAWSValidateEgressInput is an opinionated interface in front of osd-network-verifier.
// Its input is an OCM internal/external clusterId and it returns the corresponding input to osd-network-verifier with
// default AWS tags, one of the cluster's private subnet IDs, and the cluster's master security group.
// Can override securityGroupId and subnetId.
func (e *egressVerification) generateAWSValidateEgressInput(ctx context.Context, region string) (*onv.ValidateEgressInput, error) {
	// We can auto-detect information from OCM
	if e.cluster != nil {
		// TODO: osd-network-verifier technically does support GCP, but just handle AWS for now
		if e.cluster.CloudProvider().ID() != "aws" {
			return nil, fmt.Errorf("only supports aws, got %s", e.cluster.CloudProvider().ID())
		}

		if e.cluster.Product().ID() != "rosa" && e.cluster.Product().ID() != "osd" {
			return nil, fmt.Errorf("only supports rosa and osd, got %s", e.cluster.Product().ID())
		}
	}

	input, err := defaultValidateEgressInput(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble validate egress input: %s", err)
	}

	// Setup proxy configuration that is not automatically determined
	input.Proxy.NoTls = e.noTls
	if e.caCert != "" {
		cert, err := os.ReadFile(e.caCert)
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
			if e.cluster.AdditionalTrustBundle() != "" && e.caCert == "" {
				return nil, fmt.Errorf("%s has an additional trust bundle configured, but no --cacert supplied", e.clusterId)
			}
		}
	}

	// Fill in subnetID
	subnetId, err := e.getSubnetId(context.TODO())
	if err != nil {
		return nil, err
	}
	input.SubnetID = subnetId

	// Fill in securityGroupID
	sgId, err := e.getSecurityGroupId(context.TODO())
	if err != nil {
		return nil, err
	}
	input.AWS.SecurityGroupId = sgId

	return input, nil
}

// getSubnetId attempts to return a private subnet ID.
// e.subnetId acts as an override, otherwise e.awsClient will be used to attempt to determine the correct subnets
func (e *egressVerification) getSubnetId(ctx context.Context) (string, error) {
	// A subnetId was manually specified, just use that
	if e.subnetId != "" {
		e.log.Info(ctx, "using manually specified subnet-id: %s", e.subnetId)
		return e.subnetId, nil
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
			return "", fmt.Errorf("failed to find private subnets for %s: %w", e.cluster.InfraID(), err)
		}

		if len(resp.Subnets) == 0 {
			return "", fmt.Errorf("found 0 subnets with kubernetes.io/cluster/%s=owned and %s, consider the --subnet-id flag", e.cluster.InfraID(), e.cluster.InfraID())
		}

		e.log.Info(ctx, "using subnet-id: %s", *resp.Subnets[0].SubnetId)
		return *resp.Subnets[0].SubnetId, nil
	}

	// For PrivateLink clusters, any provided subnet is considered a private subnet
	if e.cluster.AWS().PrivateLink() {
		if len(e.cluster.AWS().SubnetIDs()) == 0 {
			return "", fmt.Errorf("unexpected error: %s is a PrivateLink cluster, but no subnets in OCM", e.cluster.InfraID())
		}

		e.log.Info(ctx, "detected BYOVPC PrivateLink cluster, using first subnet from OCM: %s", e.cluster.AWS().SubnetIDs()[0])
		return e.cluster.AWS().SubnetIDs()[0], nil
	}

	// For non-PrivateLink BYOVPC clusters, provided subnets are 50/50 public/private subnets, so make the user decide for now
	// TODO: Figure out via IGW/NAT GW/Route Tables
	return "", fmt.Errorf("unable to determine which non-PrivateLink BYOVPC subnets are private yet, please check manually and provide the --subnet-id flag")
}

// getSecurityGroupId attempts to return a cluster's master node security group Id
// e.securityGroupId acts as an override, otherwise e.awsClient will be used to attempt to determine the correct security group
func (e *egressVerification) getSecurityGroupId(ctx context.Context) (string, error) {
	// A securityGroupId was manually specified, just use that
	if e.securityGroupId != "" {
		e.log.Info(ctx, "using manually specified security-group-id: %s", e.securityGroupId)
		return e.securityGroupId, nil
	}

	// If no securityGroupId override is passed in, try to determine the master security group id
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
