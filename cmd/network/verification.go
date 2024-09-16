package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift-online/ocm-sdk-go/logging"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/osd-network-verifier/pkg/data/cpu"
	"github.com/openshift/osd-network-verifier/pkg/helpers"
	"github.com/openshift/osd-network-verifier/pkg/output"
	"github.com/openshift/osd-network-verifier/pkg/probes/curl"
	"github.com/openshift/osd-network-verifier/pkg/probes/legacy"
	"github.com/openshift/osd-network-verifier/pkg/proxy"
	onv "github.com/openshift/osd-network-verifier/pkg/verifier"
	onvAwsClient "github.com/openshift/osd-network-verifier/pkg/verifier/aws"
	lsupport "github.com/openshift/osdctl/cmd/cluster/support"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	nonByovpcPrivateSubnetTagKey = "kubernetes.io/role/internal-elb"
	blockedEgressTemplateUrl     = "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/required_network_egresses_are_blocked.json"
	caBundleConfigMapKey         = "ca-bundle.crt"
	networkVerifierDepPath       = "github.com/openshift/osd-network-verifier"
	LimitedSupportTemplate       = "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/limited_support/egressFailureLimitedSupport.json"
)

type EgressVerification struct {
	awsClient egressVerificationAWSClient
	cluster   *cmv1.Cluster
	log       logging.Logger

	// ClusterId is the internal or external OCM cluster ID.
	// This is optional, but typically is used to automatically detect the correct settings.
	ClusterId string
	// PlatformType is an optional override for which endpoints to test. Either 'aws' or 'hostedcluster'
	// TODO: Technically 'gcp' is supported, but not functional yet
	PlatformType string
	// AWS Region is an optional override if not specified via AWS credentials.
	Region string
	// SubnetIds is an optional override for specifying an AWS subnet ID.
	// Must be a private subnet to provide accurate results.
	SubnetIds []string
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
	// EgressTimeout The timeout to wait when testing egresses
	EgressTimeout time.Duration
	// Version Whether to print out the version of osd-network-verifier being used
	Version bool
	// Probe The type of probe to use for the verifier
	Probe string
	// CpuArchName the architecture to use for the compute instance
	CpuArchName string
	cpuArch     cpu.Architecture
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

  The osd-network-verifier launches a probe, an instance in a given subnet, and checks egress to external required URL's. Since October 2022, the probe is an instance without a public IP address. For this reason, the probe's requests will fail for subnets that don't have a NAT gateway. The osdctl network verify-egress command will always fail and give a false negative for public subnets (in non-privatelink clusters), since they have an internet gateway and no NAT gateway.

  Docs: https://docs.openshift.com/rosa/rosa_install_access_delete_clusters/rosa_getting_started_iam/rosa-aws-prereqs.html#osd-aws-privatelink-firewall-prerequisites_prerequisites`,
		Example: `
  # Run against a cluster registered in OCM
  osdctl network verify-egress --cluster-id my-rosa-cluster

  # Run against a cluster registered in OCM with a cluster-wide-proxy
  touch cacert.txt
  osdctl network verify-egress --cluster-id my-rosa-cluster --cacert cacert.txt

  # Override automatic selection of a subnet or security group id
  osdctl network verify-egress --cluster-id my-rosa-cluster --subnet-id subnet-abcd --security-group sg-abcd

  # Override automatic selection of the list of endpoints to check
  osdctl network verify-egress --cluster-id my-rosa-cluster --platform hostedcluster

  # (Not recommended) Run against a specific VPC, without specifying cluster-id
  <export environment variables like AWS_ACCESS_KEY_ID or use aws configure>
  osdctl network verify-egress --subnet-id subnet-abcdefg123 --security-group sg-abcdefgh123 --region us-east-1`,
		Run: func(cmd *cobra.Command, args []string) {
			if e.Version {
				printVersion()
			}
			e.Run(context.TODO())
		},
	}

	validateEgressCmd.Flags().StringVarP(&e.ClusterId, "cluster-id", "C", "", "(optional) OCM internal/external cluster id to run osd-network-verifier against.")
	validateEgressCmd.Flags().StringArrayVar(&e.SubnetIds, "subnet-id", nil, "(optional) private subnet ID override, required if not specifying --cluster-id and can be specified multiple times to run against multiple subnets")
	validateEgressCmd.Flags().StringVar(&e.SecurityGroupId, "security-group", "", "(optional) security group ID override for osd-network-verifier, required if not specifying --cluster-id")
	validateEgressCmd.Flags().StringVar(&e.CaCert, "cacert", "", "(optional) path to a file containing the additional CA trust bundle. Typically set so that the verifier can use a configured cluster-wide proxy.")
	validateEgressCmd.Flags().BoolVar(&e.NoTls, "no-tls", false, "(optional) if provided, ignore all ssl certificate validations on client-side.")
	validateEgressCmd.Flags().StringVar(&e.Region, "region", "", "(optional) AWS region")
	validateEgressCmd.Flags().BoolVar(&e.Debug, "debug", false, "(optional) if provided, enable additional debug-level logging")
	validateEgressCmd.Flags().BoolVarP(&e.AllSubnets, "all-subnets", "A", false, "(optional) an option for Privatelink clusters to run osd-network-verifier against all subnets listed by ocm.")
	validateEgressCmd.Flags().StringVar(&e.PlatformType, "platform", "", "(optional) override for which endpoints to test. Either 'aws' or 'hostedcluster'")
	validateEgressCmd.Flags().DurationVar(&e.EgressTimeout, "egress-timeout", 5*time.Second, "(optional) timeout for individual egress verification requests")
	validateEgressCmd.Flags().BoolVar(&e.Version, "version", false, "When present, prints out the version of osd-network-verifier being used")
	validateEgressCmd.Flags().StringVar(&e.Probe, "probe", "curl", "(optional) select the probe to be used for egress testing. Either 'curl' (default) or 'legacy'")
	validateEgressCmd.Flags().StringVar(&e.CpuArchName, "cpu-arch", "x86", "(optional) compute instance CPU architecture. E.g., 'x86' or 'arm'")

	// If a cluster-id is specified, don't allow the foot-gun of overriding region
	validateEgressCmd.MarkFlagsMutuallyExclusive("cluster-id", "region")

	return validateEgressCmd
}

type egressVerificationAWSClient interface {
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
	DescribeRouteTables(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
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
	e.log.Info(ctx, "Preparing to check %+v subnet(s) with network verifier.", len(inputs))

	for i := range inputs {
		e.log.Info(ctx, "running network verifier for subnet  %+v, security group %+v", inputs[i].SubnetID, inputs[i].AWS.SecurityGroupIDs)
		out := onv.ValidateEgress(c, *inputs[i])
		out.Summary(e.Debug)
		// Prompt putting the cluster into LS if egresses crucial for monitoring (PagerDuty/DMS) are blocked.
		// Prompt sending a service log instead for other blocked egresses.
		if !out.IsSuccessful() && len(out.GetEgressURLFailures()) > 0 {
			postCmd := generateServiceLog(out, e.ClusterId)
			blockedUrl := strings.Join(postCmd.TemplateParams, ",")
			if strings.Contains(blockedUrl, "deadmanssnitch") || strings.Contains(blockedUrl, "pagerduty") {
				fmt.Println("PagerDuty and/or DMS outgoing traffic is blocked, resulting in a loss of observability. As a result, Red Hat can no longer guarantee SLAs and the cluster should be put in limited support")
				pCmd := lsupport.Post{Template: LimitedSupportTemplate}
				if err := pCmd.Run(e.ClusterId); err != nil {
					fmt.Printf("failed to post limited support reason: %v", err)
				}
			} else if err := postCmd.Run(); err != nil {
				fmt.Println("Failed to generate service log. Please manually send a service log to the customer for the blocked egresses with:")
				fmt.Printf("osdctl servicelog post %v -t %v -p %v\n", e.ClusterId, blockedEgressTemplateUrl, strings.Join(postCmd.TemplateParams, " -p "))
			}
		}
	}
}

func generateServiceLog(out *output.Output, clusterId string) servicelog.PostCmdOptions {
	failures := out.GetEgressURLFailures()
	if len(failures) > 0 {
		egressUrls := make([]string, len(failures))
		for i, failure := range failures {
			egressUrls[i] = failure.EgressURL()
		}

		return servicelog.PostCmdOptions{
			Template:       blockedEgressTemplateUrl,
			ClusterId:      clusterId,
			TemplateParams: []string{fmt.Sprintf("URLS=%v", strings.Join(egressUrls, ","))},
		}
	}
	return servicelog.PostCmdOptions{}
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

	e.cpuArch = cpu.ArchitectureByName(e.CpuArchName)
	if e.CpuArchName != "" && !e.cpuArch.IsValid() {
		return nil, fmt.Errorf("%s is not a valid architecture", e.CpuArchName)
	}

	// If ClusterId is supplied, leverage ocm and ocm-backplane to get an AWS client
	if e.ClusterId != "" {
		e.log.Debug(ctx, "searching OCM for cluster: %s", e.ClusterId)
		ocmClient, err := utils.CreateConnection()
		if err != nil {
			return nil, err
		}
		defer ocmClient.Close()

		cluster, err := utils.GetClusterAnyStatus(ocmClient, e.ClusterId)
		if err != nil {
			return nil, fmt.Errorf("failed to get OCM cluster info for %s: %s", e.ClusterId, err)
		}
		e.log.Debug(ctx, "cluster %s found from OCM: %s", e.ClusterId, cluster.ID())
		e.cluster = cluster

		// We currently have insufficient permissions to run network verifier on ROSA HCP
		// We can update or, if applicable, remove this warning after https://issues.redhat.com/browse/XCMSTRAT-245
		if e.cluster.Hypershift().Enabled() {
			e.log.Warn(ctx, "Generally, SRE has insufficient AWS permissions"+
				" to run network verifier on hosted control plane clusters. Run anyway?")
			if !utils.ConfirmPrompt() {
				return nil, errors.New("You can try the network verifier script in ops-sop/hypershift/utils/verify-egress.sh")
			}
		}

		e.log.Info(ctx, "getting AWS credentials from backplane-api")
		cfg, err := osdCloud.CreateAWSV2Config(ocmClient, cluster)
		if err != nil {
			return nil, fmt.Errorf("failed to get credentials automatically from backplane-api: %v."+
				" You can still try this command by exporting AWS credentials as environment variables and specifying"+
				" the --subnet-id, --security-group, and other required flags manually."+
				" See osdctl network verify-egress -h for more details", err)
		}
		e.log.Debug(ctx, "retrieved AWS credentials from backplane-api")
		e.awsClient = ec2.NewFromConfig(cfg)
		return &cfg, nil
	}

	if e.SubnetIds == nil || e.SecurityGroupId == "" || e.PlatformType == "" {
		return nil, fmt.Errorf("--subnet-id, --security-group, and --platform are required when --cluster-id is not specified")
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

	e.awsClient = ec2.NewFromConfig(cfg)

	return &cfg, nil
}

// generateAWSValidateEgressInput is an opinionated interface in front of osd-network-verifier.
// Its input is an OCM internal/external ClusterId and it returns the corresponding input to osd-network-verifier with
// default AWS tags, one of the cluster's private subnet IDs, and the cluster's master security group.
// Can override SecurityGroupId and SubnetIds.
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

	input, err := defaultValidateEgressInput(ctx, e.cluster, region)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble validate egress input: %s", err)
	}

	platform, err := e.getPlatformType()
	if err != nil {
		return nil, err
	}
	e.log.Info(ctx, "using platform: %s", platform)
	input.PlatformType = platform

	input.CPUArchitecture = e.cpuArch

	// Setup proxy configuration that is not automatically determined
	input.Proxy.NoTls = e.NoTls
	if e.CaCert != "" {
		cert, err := os.ReadFile(e.CaCert)
		if err != nil {
			return nil, err
		}
		input.Proxy.Cacert = bytes.NewBuffer(cert).String()
	}

	if e.cluster != nil {
		// If a KMS key is defined for the cluster, use it as the default aws/ebs key may not exist
		clusterAWS := e.cluster.AWS()

		if clusterAWS != nil {
			if kmsKeyArn, isOk := clusterAWS.GetKMSKeyArn(); isOk {
				e.log.Info(ctx, "using KMS key defined for the cluster: %s", kmsKeyArn)
				input.AWS.KmsKeyID = kmsKeyArn
			}
		}

		// If the cluster has a cluster-wide proxy, configure it
		if e.cluster.Proxy() != nil && !e.cluster.Proxy().Empty() {
			input.Proxy.HttpProxy = e.cluster.Proxy().HTTPProxy()
			input.Proxy.HttpsProxy = e.cluster.Proxy().HTTPSProxy()
		}

		// The actual trust bundle is redacted in OCM, but is an indicator that --cacert is required
		if e.cluster.AdditionalTrustBundle() != "" && e.CaCert == "" {
			caBundle, err := e.getCABundle(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get additional CA trust bundle from hive with error: %v, consider specifying --cacert", err)
			}

			input.Proxy.Cacert = caBundle
		}
	}

	// Obtain subnet ids to run against
	subnetId, err := e.getSubnetIds(context.TODO())
	if err != nil {
		return nil, err
	}

	input.SubnetID = subnetId[0]

	checkPublic, err := e.isSubnetPublic(ctx, input.SubnetID)
	if err != nil {
		return nil, err
	}
	if checkPublic {
		return nil, fmt.Errorf("subnet %v you provided is public. The network verifier only works for private subnets. Please provide a private subnet ID", input.SubnetID)
	}

	// Obtain security group id to use
	sgId, err := e.getSecurityGroupId(context.TODO())
	if err != nil {
		return nil, err
	}
	input.AWS.SecurityGroupIDs = []string{sgId}

	// Forward the timeout
	input.Timeout = e.EgressTimeout

	switch strings.ToLower(e.Probe) {
	case "curl":
		input.Probe = curl.Probe{}
	case "legacy":
		input.Probe = legacy.Probe{}
	default:
		e.log.Info(ctx, "unrecognized probe %s - defaulting to curl probe.", e.Probe)
		input.Probe = curl.Probe{}
	}

	// Creating a slice of input values for the network-verifier to loop over.
	// All inputs are essentially equivalent except their subnet ids
	inputs := make([]*onv.ValidateEgressInput, len(subnetId))
	for i := range subnetId {
		// Copying a pointer to avoid overwriting it
		var myinput = &onv.ValidateEgressInput{}
		*myinput = *input
		inputs[i] = myinput
		inputs[i].SubnetID = subnetId[i]
	}

	return inputs, nil
}

// getPlatformType returns the platform type of a cluster, with e.PlatformType acting as an override.
func (e *EgressVerification) getPlatformType() (string, error) {
	switch e.PlatformType {
	case helpers.PlatformAWS:
		fallthrough
	case helpers.PlatformHostedCluster:
		return e.PlatformType, nil
	case "":
		if e.cluster.Hypershift().Enabled() {
			e.PlatformType = helpers.PlatformHostedCluster
			return helpers.PlatformHostedCluster, nil
		}

		if e.cluster.CloudProvider().ID() == "aws" {
			e.PlatformType = helpers.PlatformAWS
			return helpers.PlatformAWS, nil
		}

		return "", fmt.Errorf("only supports platform types: %s and %s", helpers.PlatformAWS, helpers.PlatformHostedCluster)
	default:
		return "", fmt.Errorf("only supports platform types: %s and %s", helpers.PlatformAWS, helpers.PlatformHostedCluster)
	}
}

// getSubnetIds attempts to return a private subnet ID or all private subnet IDs of the cluster.
// e.SubnetIds acts as an override, otherwise e.awsClient will be used to attempt to determine the correct subnets
func (e *EgressVerification) getSubnetIds(ctx context.Context) ([]string, error) {
	// A SubnetIds was manually specified, just use that
	if e.SubnetIds != nil {
		e.log.Info(ctx, "using manually specified subnet-id: %s", e.SubnetIds)
		return e.SubnetIds, nil
	}

	// If this is a non-BYOVPC cluster, we can find the private subnets based on the cluster and internal-elb tag
	if len(e.cluster.AWS().SubnetIDs()) == 0 {
		e.log.Info(ctx, "searching for subnets by tags: kubernetes.io/cluster/%s and %s=", e.cluster.InfraID(), nonByovpcPrivateSubnetTagKey)
		resp, err := e.awsClient.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
			Filters: []types.Filter{
				{
					Name:   aws.String("tag-key"),
					Values: []string{fmt.Sprintf("kubernetes.io/cluster/%s", e.cluster.InfraID())},
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
			return nil, fmt.Errorf("found 0 subnets with tags: kubernetes.io/cluster/%s and %s, consider the --subnet-id flag", e.cluster.InfraID(), nonByovpcPrivateSubnetTagKey)
		}
		if e.AllSubnets {
			subnets := make([]string, len(resp.Subnets))
			e.log.Debug(ctx, "Found %v subnets.", len(resp.Subnets))
			for i := range resp.Subnets {
				e.log.Debug(ctx, "Found subnet: %v", *resp.Subnets[i].SubnetId)
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
			e.log.Debug(ctx, "Assigned value to var e.SubnetIds: %v", subnets)
			return subnets, nil

		}

		e.log.Info(ctx, "detected BYOVPC PrivateLink cluster, using first subnet from OCM: %s", e.cluster.AWS().SubnetIDs()[0])
		return []string{e.cluster.AWS().SubnetIDs()[0]}, nil

	}

	// For non-PrivateLink BYOVPC clusters, provided subnets are 50/50 public/private subnets, so make the user decide for now
	// TODO: Figure out via IGW/NAT GW/Route Tables
	return nil, fmt.Errorf("unable to determine which non-PrivateLink BYOVPC subnets are private yet, please check manually and provide the --subnet-id flag")

}

// This function checks the gateway attached to the subnet and returns true if the subnet starts with igw- (for InternetGateway) and has a route to 0.0.0.0/0
func (e *EgressVerification) isSubnetPublic(ctx context.Context, subnetID string) (bool, error) {
	var routeTable string

	// Try and find a Route Table associated with the given subnet

	routeTable, err := utils.FindRouteTableForSubnetForVerification(e.awsClient, subnetID)

	// Check that the RouteTable for the subnet has a default route to 0.0.0.0/0
	describeRouteTablesOutput, err := e.awsClient.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		RouteTableIds: []string{routeTable},
	})
	if err != nil {
		return false, err
	}

	if len(describeRouteTablesOutput.RouteTables) == 0 {
		// Shouldn't happen
		return false, fmt.Errorf("no route tables found for route table id %v", routeTable)
	}

	// Checking if the attached gateway starts with igw- for Internet Gateway
	for _, route := range describeRouteTablesOutput.RouteTables[0].Routes {
		if route.GatewayId != nil && strings.HasPrefix(*route.GatewayId, "igw-") {
			// Some routes don't use CIDR blocks as targets, so this needs to be checked
			if route.DestinationCidrBlock != nil && *route.DestinationCidrBlock == "0.0.0.0/0" {
				return true, nil
			}
		}
	}

	// We haven't found a default route to the internet, so this subnet can't be public
	return false, nil
}

// findDefaultRouteTableForVPC returns the AWS Route Table ID of the VPC's default Route Table
func (e *EgressVerification) findDefaultRouteTableForVPC(ctx context.Context, vpcID string) (string, error) {
	describeRouteTablesOutput, err := e.awsClient.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []string{vpcID},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to describe route tables associated with vpc %s: %w", vpcID, err)
	}

	for _, rt := range describeRouteTablesOutput.RouteTables {
		for _, assoc := range rt.Associations {
			if *assoc.Main {
				return *rt.RouteTableId, nil
			}
		}
	}

	return "", fmt.Errorf("no default route table found for vpc: %s", vpcID)
}

// getSecurityGroupId attempts to return a cluster's master node security group id
// e.SecurityGroupId acts as an override, otherwise e.awsClient will be used to attempt to determine the correct security group
func (e *EgressVerification) getSecurityGroupId(ctx context.Context) (string, error) {
	// A SecurityGroupId was manually specified, just use that
	if e.SecurityGroupId != "" {
		e.log.Info(ctx, "using manually specified security-group-id: %s", e.SecurityGroupId)
		return e.SecurityGroupId, nil
	}

	var filters []types.Filter
	switch e.PlatformType {
	case helpers.PlatformHostedCluster:
		filters = []types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{fmt.Sprintf("%s-default-sg", e.cluster.ID())},
			},
		}
	default:
		filters = []types.Filter{
			{
				// Prior to 4.16: <infra_id>-master-sg
				// 4.16+: <infra_id>-controlplane
				Name:   aws.String("tag:Name"),
				Values: []string{fmt.Sprintf("%s-master-sg", e.cluster.InfraID()), fmt.Sprintf("%s-controlplane", e.cluster.InfraID())},
			},
		}
	}

	// If no SecurityGroupId override is passed in, try to determine the master security group id
	e.log.Info(ctx, "searching for security group by tags: %s", filtersToString(filters))
	resp, err := e.awsClient.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: filters,
	})
	if err != nil {
		return "", fmt.Errorf("failed to find security group for %s: %w", e.cluster.InfraID(), err)
	}

	if len(resp.SecurityGroups) == 0 {
		return "", fmt.Errorf("failed to find any security groups by tags: %s", filtersToString(filters))
	}

	e.log.Info(ctx, "using security-group-id: %s", *resp.SecurityGroups[0].GroupId)
	return *resp.SecurityGroups[0].GroupId, nil
}

// getCABundle retrieves a cluster's CA Trust Bundle
// The contents are available on a working cluster in a ConfigMap openshift-config/user-ca-bundle, however we get the
// contents from either the cluster's Hive (classic) or Management Cluster (HCP) to cover scenarios where the cluster
// is not ready yet.
func (e *EgressVerification) getCABundle(ctx context.Context) (string, error) {
	if e.cluster == nil || e.cluster.AdditionalTrustBundle() == "" {
		// No CA Bundle to retrieve for this cluster
		return "", nil
	}

	if e.cluster.Hypershift().Enabled() {
		e.log.Info(ctx, "cluster has an additional trusted CA bundle, but none specified - attempting to retrieve from the management cluster")
		return e.getCaBundleFromManagementCluster(ctx)
	}

	e.log.Info(ctx, "cluster has an additional trusted CA bundle, but none specified - attempting to retrieve from hive")
	return e.getCaBundleFromHive(ctx)
}

// getCaBundleFromManagementCluster returns a HCP cluster's additional trust bundle from its management cluster
func (e *EgressVerification) getCaBundleFromManagementCluster(ctx context.Context) (string, error) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return "", err
	}

	mc, err := utils.GetManagementCluster(e.cluster.ID())
	if err != nil {
		return "", err
	}

	e.log.Debug(ctx, "assembling K8s client for %s (%s)", mc.ID(), mc.Name())
	mcClient, err := k8s.New(mc.ID(), client.Options{Scheme: scheme})
	if err != nil {
		return "", err
	}

	e.log.Debug(ctx, "searching for user-ca-bundle ConfigMap")
	nsList := &corev1.NamespaceList{}
	clusterSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: map[string]string{
		"api.openshift.com/id": e.cluster.ID(),
	}})
	if err != nil {
		return "", err
	}
	if err := mcClient.List(ctx, nsList, &client.ListOptions{LabelSelector: clusterSelector}); err != nil {
		return "", err
	}
	if len(nsList.Items) != 1 {
		return "", fmt.Errorf("one namespace expected matching: api.openshift.com/id=%s, found %d", e.cluster.ID(), len(nsList.Items))
	}

	cm := &corev1.ConfigMap{}
	if err := mcClient.Get(ctx, client.ObjectKey{Name: "user-ca-bundle", Namespace: nsList.Items[0].Name}, cm); err != nil {
		return "", err
	}

	if _, ok := cm.Data[caBundleConfigMapKey]; ok {
		return cm.Data[caBundleConfigMapKey], nil
	}

	return "", fmt.Errorf("%s data not found in the ConfigMap %s/user-ca-bundle on %s", caBundleConfigMapKey, nsList.Items[0].Name, mc.Name())
}

func (e *EgressVerification) getCaBundleFromHive(ctx context.Context) (string, error) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return "", err
	}
	// Register hivev1 for SelectorSyncSets
	if err := hivev1.AddToScheme(scheme); err != nil {
		return "", err
	}

	hive, err := utils.GetHiveCluster(e.cluster.ID())
	if err != nil {
		return "", err
	}

	e.log.Debug(ctx, "assembling K8s client for %s (%s)", hive.ID(), hive.Name())
	hc, err := k8s.New(hive.ID(), client.Options{Scheme: scheme})
	if err != nil {
		return "", err
	}

	e.log.Debug(ctx, "searching for proxy SyncSet")
	nsList := &corev1.NamespaceList{}
	clusterSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: map[string]string{
		"api.openshift.com/id": e.cluster.ID(),
	}})
	if err != nil {
		return "", err
	}

	if err := hc.List(ctx, nsList, &client.ListOptions{LabelSelector: clusterSelector}); err != nil {
		return "", err
	}

	if len(nsList.Items) != 1 {
		return "", fmt.Errorf("one namespace expected matching: api.openshift.com/id=%s, found %d", e.cluster.ID(), len(nsList.Items))
	}

	ss := &hivev1.SyncSet{}
	if err := hc.Get(ctx, client.ObjectKey{Name: "proxy", Namespace: nsList.Items[0].Name}, ss); err != nil {
		return "", err
	}

	e.log.Debug(ctx, "extracting additional trusted CA bundle from proxy SyncSet")
	caBundle, err := getCaBundleFromSyncSet(ss)
	if err != nil {
		return "", err
	}

	return caBundle, nil
}

// getCaBundleFromSyncSet returns a cluster's proxy CA Bundle given a Hive SyncSet
func getCaBundleFromSyncSet(ss *hivev1.SyncSet) (string, error) {
	decoder := admission.NewDecoder(runtime.NewScheme())

	for i := range ss.Spec.Resources {
		cm := &corev1.ConfigMap{}
		if err := decoder.DecodeRaw(ss.Spec.Resources[i], cm); err != nil {
			return "", err
		}

		if _, ok := cm.Data[caBundleConfigMapKey]; ok {
			return cm.Data[caBundleConfigMapKey], nil
		}
	}

	return "", fmt.Errorf("cabundle ConfigMap not found in SyncSet: %s", ss.Name)
}

func filtersToString(filters []types.Filter) string {
	resp := make([]string, len(filters))
	for i := range filters {
		resp[i] = fmt.Sprintf("name: %s, values: %s", *filters[i].Name, strings.Join(filters[i].Values, ","))
	}

	return fmt.Sprintf("%v", resp)
}

// defaultValidateEgressInput generates an opinionated default osd-network-verifier ValidateEgressInput.
// Tags from the cluster are passed to the network-verifier instance
func defaultValidateEgressInput(ctx context.Context, cluster *cmv1.Cluster, region string) (*onv.ValidateEgressInput, error) {
	networkVerifierDefaultTags := map[string]string{
		"osd-network-verifier": "owned",
		"red-hat-managed":      "true",
		"Name":                 "osd-network-verifier",
	}

	// TODO: When this command supports GCP this will need to be adjusted
	for k, v := range cluster.AWS().Tags() {
		networkVerifierDefaultTags[k] = v
	}

	if onvAwsClient.GetAMIForRegion(region) == "" {
		return nil, fmt.Errorf("unsupported region: %s", region)
	}

	return &onv.ValidateEgressInput{
		Ctx:          ctx,
		SubnetID:     "",
		Proxy:        proxy.ProxyConfig{},
		PlatformType: helpers.PlatformAWS,
		Tags:         networkVerifierDefaultTags,
		AWS: onv.AwsEgressConfig{
			SecurityGroupIDs: []string{},
		},
	}, nil
}

func printVersion() {
	version, err := utils.GetDependencyVersion(networkVerifierDepPath)
	if err != nil {
		// This line should never be hit
		log.Fatal("Unable to find version for network verifier: %w", err)
	}
	log.Println(fmt.Sprintf("Using osd-network-verifier version %v", version))
}

// Generate LimitedSupportTemplate
func generateLimitedSupportTemplate(out *output.Output) lsupport.Post {
	failures := out.GetEgressURLFailures()
	if len(failures) > 0 {
		return lsupport.Post{
			Template: LimitedSupportTemplate,
		}
	}
	return lsupport.Post{}
}
