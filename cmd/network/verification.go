package network

import (
	"bytes"
	"context"
	"fmt"
	"github.com/openshift/osd-network-verifier/pkg/probes/curl"
	"github.com/openshift/osd-network-verifier/pkg/probes/legacy"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift-online/ocm-sdk-go/logging"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/osd-network-verifier/pkg/data/cloud"
	"github.com/openshift/osd-network-verifier/pkg/data/cpu"
	"github.com/openshift/osd-network-verifier/pkg/output"
	"github.com/openshift/osd-network-verifier/pkg/proxy"
	onv "github.com/openshift/osd-network-verifier/pkg/verifier"
	onvAwsClient "github.com/openshift/osd-network-verifier/pkg/verifier/aws"
	onvGcpClient "github.com/openshift/osd-network-verifier/pkg/verifier/gcp"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	lsupport "github.com/openshift/osdctl/cmd/cluster/support"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
)

const (
	nonByovpcPrivateSubnetTagKey = "kubernetes.io/role/internal-elb"
	blockedEgressTemplateUrl     = "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/required_network_egresses_are_blocked.json"
	caBundleConfigMapKey         = "ca-bundle.crt"
	networkVerifierDepPath       = "github.com/openshift/osd-network-verifier"
	LimitedSupportTemplate       = "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/limited_support/egressFailureLimitedSupport.json"
)

var networkVerifierDefaultTags = map[string]string{
	"osd-network-verifier": "owned",
	"red-hat-managed":      "true",
}

type EgressVerification struct {
	awsClient egressVerificationAWSClient
	cluster   *cmv1.Cluster
	cpuArch   cpu.Architecture
	log       logging.Logger

	// ClusterId is the internal or external OCM cluster ID.
	// This is optional, but typically is used to automatically detect the correct settings.
	ClusterId string
	// platformName is an optional override for the verifier's PlatformType parameter, which indicates
	// the cloud platform/product under test and controls verifier behavior like which egress URLs
	// are tested. Accepts any value accepted by cloud.ByName(). Don't read this value directly outside
	// of GetPlatform(); use that function instead
	platformName string
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
	// GcpProjectId is used in the cases where we can't automatically get this from OCM
	GcpProjectID string
	// VpcName is the VPC where the verifier will run
	VpcName string
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
			e.Run(context.Background())
		},
	}

	validateEgressCmd.Flags().StringVarP(&e.ClusterId, "cluster-id", "C", "", "(optional) OCM internal/external cluster id to run osd-network-verifier against.")
	validateEgressCmd.Flags().StringArrayVar(&e.SubnetIds, "subnet-id", nil, "(optional) private subnet ID override, required if not specifying --cluster-id and can be specified multiple times to run against multiple subnets")
	validateEgressCmd.Flags().StringVar(&e.SecurityGroupId, "security-group", "", "(optional) security group ID override for osd-network-verifier, required if not specifying --cluster-id")
	validateEgressCmd.Flags().StringVar(&e.CaCert, "cacert", "", "(optional) path to a file containing the additional CA trust bundle. Typically set so that the verifier can use a configured cluster-wide proxy.")
	validateEgressCmd.Flags().BoolVar(&e.NoTls, "no-tls", false, "(optional) if provided, ignore all ssl certificate validations on client-side.")
	validateEgressCmd.Flags().StringVar(&e.Region, "region", "", "(optional) AWS region")
	validateEgressCmd.Flags().BoolVar(&e.Debug, "debug", false, "(optional) if provided, enable additional debug-level logging")
	validateEgressCmd.Flags().BoolVarP(&e.AllSubnets, "all-subnets", "A", false, "(optional) an option for AWS Privatelink clusters to run osd-network-verifier against all subnets listed by ocm.")
	validateEgressCmd.Flags().StringVar(&e.platformName, "platform", "", "(optional) override for cloud platform/product. E.g., 'aws-classic' (OSD/ROSA Classic), 'aws-hcp' (ROSA HCP), or 'aws-hcp-zeroegress'")
	validateEgressCmd.Flags().DurationVar(&e.EgressTimeout, "egress-timeout", 5*time.Second, "(optional) timeout for individual egress verification requests")
	validateEgressCmd.Flags().BoolVar(&e.Version, "version", false, "When present, prints out the version of osd-network-verifier being used")
	validateEgressCmd.Flags().StringVar(&e.Probe, "probe", "curl", "(optional) select the probe to be used for egress testing. Either 'curl' (default) or 'legacy'")
	validateEgressCmd.Flags().StringVar(&e.CpuArchName, "cpu-arch", "x86", "(optional) compute instance CPU architecture. E.g., 'x86' or 'arm'")
	validateEgressCmd.Flags().StringVar(&e.GcpProjectID, "gcp-project-id", "", "(optional) the GCP project ID to run verification for")
	validateEgressCmd.Flags().StringVar(&e.VpcName, "vpc", "", "(optional) VPC name for cases where it can't be fetched from OCM")

	// If a cluster-id is specified, don't allow the foot-gun of overriding region
	validateEgressCmd.MarkFlagsMutuallyExclusive("cluster-id", "region")

	return validateEgressCmd
}

type networkVerifier interface {
	ValidateEgress(vei onv.ValidateEgressInput) *output.Output
	VerifyDns(vdi onv.VerifyDnsInput) *output.Output
}

// Run parses the EgressVerification input, typically sets values automatically using the ClusterId, and runs
// osd-network-verifier's egress check to validate firewall prerequisites for ROSA.
// Docs: https://docs.openshift.com/rosa/rosa_install_access_delete_clusters/rosa_getting_started_iam/rosa-aws-prereqs.html#osd-aws-privatelink-firewall-prerequisites_prerequisites
func (e *EgressVerification) Run(ctx context.Context) {
	// Setup the logger
	builder := logging.NewGoLoggerBuilder().Debug(e.Debug)
	logger, err := builder.Build()
	if err != nil {
		log.Fatalf("network verification failed to build logger: %s", err)
	}
	e.log = logger

	e.cpuArch = cpu.ArchitectureByName(e.CpuArchName)
	if e.CpuArchName != "" && !e.cpuArch.IsValid() {
		log.Fatalf("%s is not a valid CPU architecture", e.CpuArchName)
	}

	// If no ClusterId is provided, fetch from OCM
	err = e.fetchCluster(ctx)
	if err != nil {
		log.Fatal(err)
	}

	platform, err := e.getPlatform()
	if err != nil {
		log.Fatalf("error getting platform: %s", err)
	}

	var inputs []*onv.ValidateEgressInput
	var verifier networkVerifier

	switch platform {
	case cloud.AWSHCP, cloud.AWSHCPZeroEgress, cloud.AWSClassic:
		cfg, err := e.setupForAws(ctx)
		if err != nil {
			log.Fatal(err)
		}

		verifier, err = onvAwsClient.NewAwsVerifierFromConfig(*cfg, e.log)
		if err != nil {
			log.Fatalf("failed to assemble osd-network-verifier client: %s", err)
		}

		inputs, err = e.generateAWSValidateEgressInput(ctx, platform)
		if err != nil {
			log.Fatal(err)
		}
	case cloud.GCPClassic:
		credentials, err := e.setupForGcp(ctx)
		if err != nil {
			log.Fatal(err)
		}

		verifier, err = onvGcpClient.NewGcpVerifier(credentials, e.Debug)
		if err != nil {
			log.Fatalf("failed to assemble osd-network-verifier client: %s", err)
		}

		inputs, err = e.generateGcpValidateEgressInput(ctx, platform)
		if err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unsupported platform: %s", platform)
	}

	e.log.Info(ctx, "Preparing to check %+v subnet(s) with network verifier.", len(inputs))
	var failures int
	for i := range inputs {
		e.log.Info(ctx, "running network verifier for subnet  %+v, security group %+v", inputs[i].SubnetID, inputs[i].AWS.SecurityGroupIDs)
		out := onv.ValidateEgress(verifier, *inputs[i])
		out.Summary(e.Debug)
		// Prompt putting the cluster into LS if egresses crucial for monitoring (PagerDuty/DMS) are blocked.
		// Prompt sending a service log instead for other blocked egresses.
		if !out.IsSuccessful() && len(out.GetEgressURLFailures()) > 0 {
			failures++
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
		if failures > 0 {
			os.Exit(1)
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

// getPlatform returns a cloud.Platform struct corresponding to the cluster's cloud platform
// reported by OCM or to the e.platformName override string specified by the user
func (e *EgressVerification) getPlatform() (cloud.Platform, error) {
	platform, err := cloud.ByName(e.platformName)
	if err != nil {
		// Fail if user explicitly specified an invalid platform
		if e.platformName != "" {
			return platform, fmt.Errorf("network verifier rejected platform request: %w", err)
		}

		// Choose sane defaults if no platform specified by user
		if e.cluster.Hypershift().Enabled() {
			return cloud.AWSHCP, nil
		}
		if e.cluster.CloudProvider().ID() == "aws" {
			return cloud.AWSClassic, nil
		}
		if e.cluster.CloudProvider().ID() == "gcp" {
			return cloud.GCPClassic, nil
		}
	}

	return platform, err
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
func (e *EgressVerification) defaultValidateEgressInput(ctx context.Context, platform cloud.Platform) (*onv.ValidateEgressInput, error) {
	input := &onv.ValidateEgressInput{
		Ctx:             ctx,
		CPUArchitecture: e.cpuArch,
		PlatformType:    platform,
		Proxy: proxy.ProxyConfig{
			NoTls: e.NoTls,
		},
		Timeout: e.EgressTimeout,
		Tags:    networkVerifierDefaultTags,
	}

	switch strings.ToLower(e.Probe) {
	case "curl":
		input.Probe = curl.Probe{}
	case "legacy":
		input.Probe = legacy.Probe{}
	default:
		e.log.Info(ctx, "unrecognized probe %s - defaulting to curl probe.", e.Probe)
		input.Probe = curl.Probe{}
	}

	if e.cluster != nil {
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

	// Setup proxy configuration that is not automatically determined
	if e.CaCert != "" {
		cert, err := os.ReadFile(e.CaCert)
		if err != nil {
			return nil, err
		}
		input.Proxy.Cacert = bytes.NewBuffer(cert).String()
	}

	return input, nil
}

func (e *EgressVerification) fetchCluster(ctx context.Context) error {
	if e.ClusterId != "" {
		ocmClient, err := utils.CreateConnection()
		if err != nil {
			log.Fatalf("error creating OCM connection: %s", err)
		}
		defer ocmClient.Close()

		cluster, err := utils.GetClusterAnyStatus(ocmClient, e.ClusterId)
		if err != nil {
			return fmt.Errorf("failed to get OCM cluster info for %s: %s", e.ClusterId, err)
		}
		e.log.Debug(ctx, "cluster %s found from OCM: %s", e.ClusterId, cluster.ID())
		e.cluster = cluster

		switch e.cluster.Product().ID() {
		case "rosa", "osd", "osdtrial":
			break
		default:
			log.Fatalf("only supports rosa, osd, and osdtrial, got %s", e.cluster.Product().ID())
		}
	}

	return nil
}

func printVersion() {
	version, err := utils.GetDependencyVersion(networkVerifierDepPath)
	if err != nil {
		panic(fmt.Errorf("unable to find version for network verifier: %w", err))
	}
	log.Println(fmt.Sprintf("Using osd-network-verifier version %v", version))
}
