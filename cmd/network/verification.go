package network

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/openshift/osd-network-verifier/pkg/probes/curl"
	"github.com/openshift/osd-network-verifier/pkg/probes/legacy"

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
	onvKubeClient "github.com/openshift/osd-network-verifier/pkg/verifier/kube"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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
	limitedSupportTemplate       = "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/limited_support/egressFailureLimitedSupport.json"
	serviceAccountTokenPath      = "/var/run/secrets/kubernetes.io/serviceaccount/token" // #nosec G101 -- This is a standard Kubernetes ServiceAccount token path, not a credential
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
	// PodMode enables Kubernetes pod-based verification instead of cloud instances
	PodMode bool
	// KubeConfig is the path to the kubeconfig file for pod mode
	KubeConfig string
	// Namespace is the Kubernetes namespace to run verification pods in
	Namespace string
	// NoServiceLog disables automatic service log prompting on verification failures
	NoServiceLog bool
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

  The osd-network-verifier supports two modes:
  1. Traditional mode: launches a probe instance in a given subnet and checks egress to external required URLs.
     Since October 2022, the probe is an instance without a public IP address. For this reason, the probe's requests
     will fail for subnets that don't have a NAT gateway. This mode will always fail and give a false negative for
     public subnets (in non-privatelink clusters), since they have an internet gateway and no NAT gateway.
  2. Pod mode (--pod-mode): runs verification as Kubernetes Jobs within the target cluster. This mode requires
     cluster admin access but provides more accurate results as it tests from within the actual cluster environment.
     
     Pod mode uses the following Kubernetes client configuration priority:
     1. In-cluster configuration (when ServiceAccount token exists)
     2. Backplane credentials (when --cluster-id is provided)
     3. User-provided kubeconfig (when --kubeconfig is specified)
     4. Default kubeconfig (from ~/.kube/config)

  Docs: https://docs.openshift.com/rosa/rosa_install_access_delete_clusters/rosa_getting_started_iam/rosa-aws-prereqs.html#osd-aws-privatelink-firewall-prerequisites_prerequisites`,
		Example: `
  # Run against a cluster registered in OCM
  osdctl network verify-egress --cluster-id my-rosa-cluster

  # Run against a cluster registered in OCM with a cluster-wide-proxy
  touch cacert.txt
  osdctl network verify-egress --cluster-id my-rosa-cluster --cacert cacert.txt

  # Override automatic selection of a subnet or security group id
  osdctl network verify-egress --cluster-id my-rosa-cluster --subnet-id subnet-abcd --security-group sg-abcd

  # Run against multiple manually supplied subnet IDs
  osdctl network verify-egress --cluster-id my-rosa-cluster --subnet-id subnet-abcd --subnet-id subnet-efgh

  # Override automatic selection of the list of endpoints to check
  osdctl network verify-egress --cluster-id my-rosa-cluster --platform hostedcluster

  # Run in pod mode using Kubernetes jobs (requires cluster access)
  osdctl network verify-egress --cluster-id my-rosa-cluster --pod-mode

  # Run in pod mode using ServiceAccount (when running inside a Kubernetes Pod)
  osdctl network verify-egress --pod-mode --region us-east-1 --namespace my-namespace

  # Run in pod mode with custom namespace and kubeconfig
  osdctl network verify-egress --pod-mode --region us-east-1 --namespace my-namespace --kubeconfig ~/.kube/config

  # Run network verification without sending service logs on failure
  osdctl network verify-egress --cluster-id my-rosa-cluster --skip-service-log

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
	validateEgressCmd.Flags().StringVar(&e.Region, "region", "", "(optional) AWS region, required for --pod-mode if not passing a --cluster-id")
	validateEgressCmd.Flags().BoolVar(&e.Debug, "debug", false, "(optional) if provided, enable additional debug-level logging")
	validateEgressCmd.Flags().BoolVarP(&e.AllSubnets, "all-subnets", "A", false, "(optional) an option for AWS Privatelink clusters to run osd-network-verifier against all subnets listed by ocm.")
	validateEgressCmd.Flags().StringVar(&e.platformName, "platform", "", "(optional) override for cloud platform/product. E.g., 'aws-classic' (OSD/ROSA Classic), 'aws-hcp' (ROSA HCP), or 'aws-hcp-zeroegress'")
	validateEgressCmd.Flags().DurationVar(&e.EgressTimeout, "egress-timeout", onv.DefaultTimeout, "(optional) timeout for individual egress verification requests")
	validateEgressCmd.Flags().BoolVar(&e.Version, "version", false, "When present, prints out the version of osd-network-verifier being used")
	validateEgressCmd.Flags().StringVar(&e.Probe, "probe", "curl", "(optional) select the probe to be used for egress testing. Either 'curl' (default) or 'legacy'")
	validateEgressCmd.Flags().StringVar(&e.CpuArchName, "cpu-arch", "x86", "(optional) compute instance CPU architecture. E.g., 'x86' or 'arm'")
	validateEgressCmd.Flags().StringVar(&e.GcpProjectID, "gcp-project-id", "", "(optional) the GCP project ID to run verification for")
	validateEgressCmd.Flags().StringVar(&e.VpcName, "vpc", "", "(optional) VPC name for cases where it can't be fetched from OCM")
	validateEgressCmd.Flags().BoolVar(&e.PodMode, "pod-mode", false, "(optional) run verification using Kubernetes pods instead of cloud instances")
	validateEgressCmd.Flags().StringVar(&e.KubeConfig, "kubeconfig", "", "(optional) path to kubeconfig file for pod mode (uses default kubeconfig if not specified)")
	validateEgressCmd.Flags().StringVar(&e.Namespace, "namespace", "openshift-network-diagnostics", "(optional) Kubernetes namespace to run verification pods in")
	validateEgressCmd.Flags().BoolVar(&e.NoServiceLog, "skip-service-log", false, "(optional) disable automatic service log sending when verification fails")

	// Pod mode is incompatible with cloud-specific configuration flags
	validateEgressCmd.MarkFlagsMutuallyExclusive("pod-mode", "cacert")
	validateEgressCmd.MarkFlagsMutuallyExclusive("pod-mode", "subnet-id")
	validateEgressCmd.MarkFlagsMutuallyExclusive("pod-mode", "security-group")
	validateEgressCmd.MarkFlagsMutuallyExclusive("pod-mode", "all-subnets")
	validateEgressCmd.MarkFlagsMutuallyExclusive("pod-mode", "cpu-arch")
	validateEgressCmd.MarkFlagsMutuallyExclusive("pod-mode", "gcp-project-id")
	validateEgressCmd.MarkFlagsMutuallyExclusive("pod-mode", "vpc")

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

	if err := e.validateInput(); err != nil {
		log.Fatalf("network verification failed to validate input: %s", err)
	}

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

	// Setup verifier and inputs based on mode
	var inputs []*onv.ValidateEgressInput
	var verifier networkVerifier

	if e.PodMode {
		e.log.Info(ctx, "Preparing to run pod-based network verification in namespace %s.", e.Namespace)
		verifier, inputs, err = e.setupPodModeVerification(ctx, platform)
	} else {
		verifier, inputs, err = e.setupCloudProviderVerification(ctx, platform)
		e.log.Info(ctx, "Preparing to check %+v subnet(s) with network verifier.", len(inputs))
	}

	if err != nil {
		log.Fatal(err)
	}

	var failures int
	for i := range inputs {
		if !e.PodMode {
			e.log.Info(ctx, "running network verifier for subnet  %+v, security group %+v", inputs[i].SubnetID, inputs[i].AWS.SecurityGroupIDs)
		}

		out := onv.ValidateEgress(verifier, *inputs[i])
		out.Summary(e.Debug)
		// Prompt putting the cluster into LS if egresses crucial for monitoring (PagerDuty/DMS) are blocked.
		// Prompt sending a service log instead for other blocked egresses.
		if !out.IsSuccessful() && len(out.GetEgressURLFailures()) > 0 {
			failures++

			// Only send service logs if not disabled by flag
			if !e.NoServiceLog {
				postCmd := generateServiceLog(out, e.ClusterId)
				blockedUrl := strings.Join(postCmd.TemplateParams, ",")
				if (strings.Contains(blockedUrl, "deadmanssnitch") || strings.Contains(blockedUrl, "pagerduty")) && e.cluster.State() == "ready" {
					fmt.Println("PagerDuty and/or DMS outgoing traffic is blocked, resulting in a loss of observability. As a result, Red Hat can no longer guarantee SLAs and the cluster should be put in limited support")
					pCmd := lsupport.Post{Template: limitedSupportTemplate}
					if err := pCmd.Run(e.ClusterId); err != nil {
						fmt.Printf("failed to post limited support reason: %v", err)
					}
				} else if err := postCmd.Run(); err != nil {
					fmt.Println("Failed to generate service log. Please manually send a service log to the customer for the blocked egresses with:")
					fmt.Printf("osdctl servicelog post %v -t %v -p %v\n", e.ClusterId, blockedEgressTemplateUrl, strings.Join(postCmd.TemplateParams, " -p "))
				}
			} else {
				fmt.Println("Service log sending disabled by --skip-service-log flag. Network verification failed but no service log will be sent.")
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

func (e *EgressVerification) validateInput() error {
	// Validate proper usage of --subnet-id flag
	if len(e.SubnetIds) == 1 && len(strings.Split(e.SubnetIds[0], ",")) > 1 {
		return fmt.Errorf("multiple subnets passed to a single --subnet-id flag, you must pass the flag per subnet, eg " +
			"--subnet-id foo --subnet-id bar")
	}

	// Pod mode validation
	if e.PodMode {
		// Require cluster-id or explicit platform for platform determination
		if e.ClusterId == "" && e.platformName == "" {
			return fmt.Errorf("pod mode requires either --cluster-id or --platform to determine platform type")
		}

		// For AWS platforms without cluster-id, require region
		if e.ClusterId == "" && e.Region == "" {
			// Check if we're dealing with an AWS platform
			if strings.HasPrefix(strings.ToLower(e.platformName), "aws") {
				return fmt.Errorf("pod mode for AWS platforms requires --region when --cluster-id is not specified")
			}
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

// getRestConfig retrieves a Kubernetes REST config using the following priority order:
// 1. User-provided kubeconfig (when --kubeconfig is specified)
// 2. Backplane credentials (when --cluster-id is provided)
// 3. In-cluster configuration (when ServiceAccount token exists and no explicit config provided)
// 4. Default kubeconfig (from ~/.kube/config)
func (e *EgressVerification) getRestConfig(ctx context.Context) (*rest.Config, error) {
	var restConfig *rest.Config

	// Priority 1: Use explicitly provided kubeconfig
	if e.KubeConfig != "" {
		restConfig, err := clientcmd.BuildConfigFromFlags("", e.KubeConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build kubeconfig from %s: %w", e.KubeConfig, err)
		}
		e.log.Info(ctx, "Pod mode using provided kubeconfig: %s", e.KubeConfig)
		return restConfig, nil
	} else if e.ClusterId != "" {
		// Priority 2: Use backplane credentials when cluster ID is available
		restConfig, err := k8s.NewRestConfig(e.ClusterId)
		if err != nil {
			return nil, fmt.Errorf("failed to get REST config from backplane for cluster %s: %w", e.ClusterId, err)
		}
		e.log.Info(ctx, "Pod mode using backplane credentials for cluster: %s", e.ClusterId)
		return restConfig, nil
	} else if _, err := os.Stat(serviceAccountTokenPath); err == nil {
		// Priority 3: Try in-cluster configuration when no explicit config provided
		var err error
		restConfig, err = rest.InClusterConfig()
		if err == nil {
			e.log.Info(ctx, "Pod mode using in-cluster configuration with ServiceAccount")
			return restConfig, nil
		} else {
			e.log.Info(ctx, "ServiceAccount token found but in-cluster config failed, falling back to default kubeconfig")
		}
	}

	// Priority 4: Fallback to default kubeconfig from environment or home directory
	kubeconfig := clientcmd.RecommendedHomeFile
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build default kubeconfig: %w", err)
	}
	e.log.Info(ctx, "Pod mode using default kubeconfig")
	return restConfig, nil
}

// setupForPodMode creates a Kubernetes client and KubeVerifier for pod-based verification
func (e *EgressVerification) setupForPodMode(ctx context.Context) (*onvKubeClient.KubeVerifier, error) {
	restConfig, err := e.getRestConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Create Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Create KubeVerifier
	kubeVerifier, err := onvKubeClient.NewKubeVerifier(clientset, e.Debug)
	if err != nil {
		return nil, fmt.Errorf("failed to create KubeVerifier: %w", err)
	}

	// Set namespace if specified
	if e.Namespace != "" {
		kubeVerifier.KubeClient.SetNamespace(e.Namespace)
	}

	e.log.Info(ctx, "Pod mode initialized with namespace: %s", e.Namespace)
	return kubeVerifier, nil
}

// setupPodModeVerification sets up pod-based verification and returns verifier and inputs
func (e *EgressVerification) setupPodModeVerification(ctx context.Context, platform cloud.Platform) (networkVerifier, []*onv.ValidateEgressInput, error) {
	// Force curl probe for pod mode
	if strings.ToLower(e.Probe) != "curl" {
		e.log.Info(ctx, "Pod mode only supports curl probe, switching from %s to curl", e.Probe)
		e.Probe = "curl"
	}

	verifier, err := e.setupForPodMode(ctx)
	if err != nil {
		return nil, nil, err
	}

	input, err := e.defaultValidateEgressInput(ctx, platform)
	if err != nil {
		return nil, nil, err
	}

	// For AWS-based platforms in pod mode, ensure region is set for proper egress list generation
	if platform == cloud.AWSClassic || platform == cloud.AWSHCP || platform == cloud.AWSHCPZeroEgress {
		var region string

		// Try to detect region from OCM cluster info first
		if e.cluster != nil && e.cluster.Region() != nil && e.cluster.Region().ID() != "" {
			region = e.cluster.Region().ID()
			e.log.Info(ctx, "Detected AWS region from OCM: %s", region)
		} else if e.Region != "" {
			// Use manually specified region
			region = e.Region
			e.log.Info(ctx, "Using manually specified AWS region: %s", region)
		} else {
			// No region available - require user to specify it
			return nil, nil, fmt.Errorf("pod mode for AWS platforms requires region information. Please specify --region or provide --cluster-id for automatic detection")
		}

		// Set AWS config in the input for region-specific egress list generation
		input.AWS = onv.AwsEgressConfig{
			Region: region,
		}
	}

	// For pod mode, we only need one input since we're not dealing with multiple subnets
	inputs := []*onv.ValidateEgressInput{input}
	return verifier, inputs, nil
}

// setupCloudProviderVerification sets up cloud provider-based verification and returns verifier and inputs
func (e *EgressVerification) setupCloudProviderVerification(ctx context.Context, platform cloud.Platform) (networkVerifier, []*onv.ValidateEgressInput, error) {
	switch platform {
	case cloud.AWSHCP, cloud.AWSHCPZeroEgress, cloud.AWSClassic:
		cfg, err := e.setupForAws(ctx)
		if err != nil {
			return nil, nil, err
		}

		verifier, err := onvAwsClient.NewAwsVerifierFromConfig(*cfg, e.log)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to assemble osd-network-verifier client: %s", err)
		}

		inputs, err := e.generateAWSValidateEgressInput(ctx, platform)
		if err != nil {
			return nil, nil, err
		}
		return verifier, inputs, nil

	case cloud.GCPClassic:
		credentials, err := e.setupForGcp(ctx)
		if err != nil {
			return nil, nil, err
		}

		verifier, err := onvGcpClient.NewGcpVerifier(credentials, e.Debug)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to assemble osd-network-verifier client: %s", err)
		}

		inputs, err := e.generateGcpValidateEgressInput(ctx, platform)
		if err != nil {
			return nil, nil, err
		}
		return verifier, inputs, nil

	default:
		return nil, nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}
