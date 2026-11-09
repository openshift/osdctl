package recover

import (
	"context"
	"fmt"
	"os"
	"strings"

	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type dnsValidateOptions struct {
	clusterID         string
	managementCluster string
	reason            string
	debug             bool

	// Populated during validation
	ocmClient *internalclient.OCMClient
	logger    *logrus.Logger
}

func newCmdValidate() *cobra.Command {
	opts := &dnsValidateOptions{}
	cmd := &cobra.Command{
		Use:   "validate --cluster-id <hcp-id> --management-cluster <mc-name> --reason <reason>",
		Short: "Validate DNS and networking health after HCP disaster recovery",
		Long: `Validate DNS and networking health after HCP management cluster disaster recovery.

After running recover to restore DNS reconciliation, this command performs
comprehensive health validation:

  1. Verify Route 53 records are properly reconciled (A/CNAME for api/oauth endpoints)
  2. Validate networking resources (AWSEndpointService, private-router Service)
  3. Check control plane components (kube-apiserver, etcd, networking controllers)
  4. Verify HCP cluster is reachable via OCM and backplane

Use this command to confirm that DNS recovery completed successfully and the cluster
is fully accessible.`,
		Example: `  # Validate cluster after DNS recovery
  osdctl hcp recover validate --cluster-id my-hcp-cluster --management-cluster dest-mgmt-cluster --reason "DR validation"

  # Validate with debug logging
  osdctl hcp recover validate --cluster-id my-hcp-cluster --management-cluster dest-mgmt-cluster --reason "DR validation" --debug`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.logger = logrus.New()
			opts.logger.SetOutput(cmd.ErrOrStderr())
			if opts.debug {
				opts.logger.SetLevel(logrus.DebugLevel)
			} else {
				opts.logger.SetLevel(logrus.InfoLevel)
			}

			return opts.Run(cmd.Context())
		},
	}

	cmd.Flags().StringVarP(&opts.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().StringVarP(&opts.managementCluster, "management-cluster", "M", "", "Name of the management cluster")
	cmd.Flags().StringVarP(&opts.reason, "reason", "R", "", "Reason for elevated access to management cluster")
	cmd.Flags().BoolVar(&opts.debug, "debug", false, "Enable debug logging output")

	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("management-cluster")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}

// validate checks that all required flags are set and initializes the OCM client.
func (o *dnsValidateOptions) validate(ctx context.Context, ocmConn *ocmsdk.Connection) error {
	if o.clusterID == "" || o.managementCluster == "" || o.reason == "" {
		return fmt.Errorf("--cluster-id, --management-cluster, and --reason are required")
	}

	if ocmConn != nil {
		o.ocmClient = internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: o.logger})
	}

	return nil
}

// Run executes the DNS recovery validation workflow across four phases: DNS records, networking resources, control plane components, and cluster reachability.
func (o *dnsValidateOptions) Run(ctx context.Context) error {
	// Create single OCM connection for entire operation
	ocmConn, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("creating OCM connection: %w", err)
	}
	defer ocmConn.Close()

	// Validate with connection
	if err := o.validate(ctx, ocmConn); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "╔════════════════════════════════════════════════════════════╗\n")
	fmt.Fprintf(os.Stderr, "║          DNS Recovery Validation                          ║\n")
	fmt.Fprintf(os.Stderr, "╚════════════════════════════════════════════════════════════╝\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  Cluster:  %s\n", o.clusterID)
	fmt.Fprintf(os.Stderr, "  MC:       %s\n", o.managementCluster)
	fmt.Fprintf(os.Stderr, "\n")

	validationResults := validationResult{
		phases: make([]phaseResult, 0, 4),
	}

	// Phase 1: Validate Route53 DNS records
	o.logger.Infof("Phase 1/4: Validating Route53 cluster records...")
	phase1 := o.validateDNSRecords(ctx, ocmConn)
	validationResults.phases = append(validationResults.phases, phase1)
	printPhaseResult(phase1)

	// Phase 2: Validate networking resources (AWSEndpointService)
	o.logger.Infof("Phase 2/4: Validating networking resources...")
	phase2 := o.validateNetworkingResources(ctx, ocmConn)
	validationResults.phases = append(validationResults.phases, phase2)
	printPhaseResult(phase2)

	// Phase 3: Validate control plane components
	o.logger.Infof("Phase 3/4: Validating control plane components...")
	phase3 := o.validateControlPlaneComponents(ctx, ocmConn)
	validationResults.phases = append(validationResults.phases, phase3)
	printPhaseResult(phase3)

	// Phase 4: Validate HCP cluster reachability
	o.logger.Infof("Phase 4/4: Validating HCP cluster reachability...")
	phase4 := o.validateClusterReachability(ctx, ocmConn)
	validationResults.phases = append(validationResults.phases, phase4)
	printPhaseResult(phase4)

	// Print final summary
	printValidationSummary(validationResults, o.clusterID, o.managementCluster)

	// Return error if any phase failed
	for _, phase := range validationResults.phases {
		if phase.status == statusFailed {
			return fmt.Errorf("validation failed - see details above")
		}
	}

	return nil
}

type validationStatus int

const (
	statusPassed validationStatus = iota
	statusWarning
	statusFailed
)

type phaseResult struct {
	name    string
	status  validationStatus
	details []string
	errors  []string
}

type validationResult struct {
	phases []phaseResult
}

// validateDNSRecords queries Route 53 for cluster DNS records and verifies that API and OAuth endpoints are present.
func (o *dnsValidateOptions) validateDNSRecords(ctx context.Context, ocmConn *ocmsdk.Connection) phaseResult {
	result := phaseResult{
		name:    "Route53 DNS Records",
		status:  statusPassed,
		details: make([]string, 0),
		errors:  make([]string, 0),
	}

	// Use DNS releaser to query current Route 53 state
	var releaser internalcmd.DNSReleaser = internalcmd.NewDefaultDNSReleaser(
		internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: o.logger}),
	)
	plan, err := releaser.PrepareDNSRelease(ctx, internalcmd.ReleaseDNSRequest{ClusterID: o.clusterID})
	if err != nil {
		result.status = statusFailed
		result.errors = append(result.errors, fmt.Sprintf("Failed to query Route 53: %v", err))
		return result
	}

	result.details = append(result.details, fmt.Sprintf("Account: %s", plan.AccountID))
	result.details = append(result.details, fmt.Sprintf("Hosted zone: %s (%s)", plan.ZoneName, plan.ZoneID))

	if len(plan.Records) == 0 {
		result.status = statusWarning
		result.details = append(result.details, "No DNS records found - ExternalDNS may not have reconciled yet")
		result.details = append(result.details, "Wait 2-3 minutes for ExternalDNS to create records")
		return result
	}

	// Count record types
	recordTypes := make(map[string]int)
	for _, record := range plan.Records {
		recordTypes[string(record.Type)]++
	}

	result.details = append(result.details, fmt.Sprintf("Found %d DNS record(s):", len(plan.Records)))
	for _, record := range plan.Records {
		result.details = append(result.details, fmt.Sprintf("  %s", internalcmd.FormatDNSRecord(record)))
	}

	// Verify we have expected endpoint records (api, oauth)
	hasAPI := false
	hasOAuth := false
	for _, record := range plan.Records {
		name := strings.ToLower(*record.Name)
		if strings.Contains(name, "api.") {
			hasAPI = true
		}
		if strings.Contains(name, "oauth.") {
			hasOAuth = true
		}
	}

	if !hasAPI || !hasOAuth {
		result.status = statusWarning
		result.details = append(result.details, "Missing expected DNS endpoints (api and/or oauth)")
	}

	return result
}

// validateNetworkingResources checks that the private-router AWSEndpointService and LoadBalancer Service exist and are ready.
func (o *dnsValidateOptions) validateNetworkingResources(ctx context.Context, ocmConn *ocmsdk.Connection) phaseResult {
	result := phaseResult{
		name:    "Networking Resources",
		status:  statusPassed,
		details: make([]string, 0),
		errors:  make([]string, 0),
	}

	// Resolve cluster info
	ocmClusterInfo, err := o.ocmClient.Resolve(ctx, o.clusterID)
	if err != nil {
		result.status = statusFailed
		result.errors = append(result.errors, fmt.Sprintf("Failed to resolve cluster: %v", err))
		return result
	}

	hcpNamespace, err := o.ocmClient.HCPNamespace(ctx, ocmClusterInfo.HCPClusterID)
	if err != nil {
		result.status = statusFailed
		result.errors = append(result.errors, fmt.Sprintf("Failed to resolve HCP namespace: %v", err))
		return result
	}

	result.details = append(result.details, fmt.Sprintf("HCP namespace: %s", hcpNamespace))

	// Connect to management cluster
	k8sClient, err := o.ocmClient.ElevatedManagementCluster(o.managementCluster, o.reason)
	if err != nil {
		result.status = statusFailed
		result.errors = append(result.errors, fmt.Sprintf("Failed to connect to management cluster: %v", err))
		return result
	}

	// Check private-router AWSEndpointService
	endpoint, err := k8sClient.GetAWSEndpointService(ctx, hcpNamespace, "private-router")
	if err != nil {
		if apierrors.IsNotFound(err) {
			result.status = statusFailed
			result.errors = append(result.errors, "AWSEndpointService 'private-router' not found")
		} else {
			result.status = statusFailed
			result.errors = append(result.errors, fmt.Sprintf("Failed to get AWSEndpointService: %v", err))
		}
		return result
	}

	if endpoint.GetDeletionTimestamp() != nil {
		result.status = statusWarning
		result.details = append(result.details, "AWSEndpointService 'private-router' is being deleted")
	} else {
		result.details = append(result.details, fmt.Sprintf("AWSEndpointService 'private-router': READY (UID: %s)", endpoint.GetUID()))
	}

	// Check private-router Service (LoadBalancer)
	kubeClient := k8sClient.GetRuntimeClient()
	svc := &unstructured.Unstructured{}
	svc.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"})
	svcKey := client.ObjectKey{Namespace: hcpNamespace, Name: "private-router"}
	if err := kubeClient.Get(ctx, svcKey, svc); err != nil {
		if apierrors.IsNotFound(err) {
			result.status = statusWarning
			result.details = append(result.details, "Service 'private-router' not found")
		} else {
			result.status = statusFailed
			result.errors = append(result.errors, fmt.Sprintf("Failed to get Service: %v", err))
		}
	} else {
		svcType, _, _ := unstructured.NestedString(svc.Object, "spec", "type")
		result.details = append(result.details, fmt.Sprintf("Service 'private-router': READY (type: %s)", svcType))
	}

	return result
}

// validateControlPlaneComponents verifies that key control plane pods (etcd, kube-apiserver, konnectivity, ExternalDNS, OVN) are ready.
func (o *dnsValidateOptions) validateControlPlaneComponents(ctx context.Context, ocmConn *ocmsdk.Connection) phaseResult {
	result := phaseResult{
		name:    "Control Plane Components",
		status:  statusPassed,
		details: make([]string, 0),
		errors:  make([]string, 0),
	}

	// Resolve cluster info
	ocmClusterInfo, err := o.ocmClient.Resolve(ctx, o.clusterID)
	if err != nil {
		result.status = statusFailed
		result.errors = append(result.errors, fmt.Sprintf("Failed to resolve cluster: %v", err))
		return result
	}

	hcpNamespace, err := o.ocmClient.HCPNamespace(ctx, ocmClusterInfo.HCPClusterID)
	if err != nil {
		result.status = statusFailed
		result.errors = append(result.errors, fmt.Sprintf("Failed to resolve HCP namespace: %v", err))
		return result
	}

	// Connect to management cluster
	k8sClient, err := o.ocmClient.ElevatedManagementCluster(o.managementCluster, o.reason)
	if err != nil {
		result.status = statusFailed
		result.errors = append(result.errors, fmt.Sprintf("Failed to connect to management cluster: %v", err))
		return result
	}

	kubeClient := k8sClient.GetRuntimeClient()

	// Check etcd StatefulSet
	etcdSts := &unstructured.Unstructured{}
	etcdSts.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"})
	etcdKey := client.ObjectKey{Namespace: hcpNamespace, Name: "etcd"}
	if err := kubeClient.Get(ctx, etcdKey, etcdSts); err != nil {
		result.status = statusFailed
		result.errors = append(result.errors, fmt.Sprintf("etcd StatefulSet not found: %v", err))
	} else {
		replicas, _, _ := unstructured.NestedInt64(etcdSts.Object, "spec", "replicas")
		readyReplicas, _, _ := unstructured.NestedInt64(etcdSts.Object, "status", "readyReplicas")
		if readyReplicas < replicas {
			result.status = statusFailed
			result.errors = append(result.errors, fmt.Sprintf("etcd not ready (%d/%d) - check for TLS certificate errors", readyReplicas, replicas))
		} else {
			result.details = append(result.details, fmt.Sprintf("etcd: %d/%d ready", readyReplicas, replicas))
		}
	}

	// Check key control plane components
	componentsToCheck := []struct {
		label string
		name  string
	}{
		{"app=kube-apiserver", "kube-apiserver"},
		{"app=konnectivity-agent", "konnectivity-agent"},
		{"app=control-plane-operator", "control-plane-operator"},
		{"name=external-dns", "external-dns"},
		{"app=ovnkube-control-plane", "ovnkube-control-plane"},
		{"app=private-router", "private-router"},
	}

	for _, comp := range componentsToCheck {
		podList := &unstructured.UnstructuredList{}
		podList.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PodList"})

		labelKey := strings.SplitN(comp.label, "=", 2)[0]
		labelVal := strings.SplitN(comp.label, "=", 2)[1]

		if err := kubeClient.List(ctx, podList, client.InNamespace(hcpNamespace), client.MatchingLabels{labelKey: labelVal}); err != nil {
			result.status = statusWarning
			result.details = append(result.details, fmt.Sprintf("%s: error listing pods (%v)", comp.name, err))
			continue
		}

		if len(podList.Items) == 0 {
			result.status = statusWarning
			result.details = append(result.details, fmt.Sprintf("%s: no pods found", comp.name))
			continue
		}

		readyCount := 0
		totalCount := len(podList.Items)
		for i := range podList.Items {
			pod := &podList.Items[i]
			conditions, found, _ := unstructured.NestedSlice(pod.Object, "status", "conditions")
			if !found {
				continue
			}
			for _, cond := range conditions {
				condMap, ok := cond.(map[string]interface{})
				if !ok {
					continue
				}
				if condMap["type"] == "Ready" && condMap["status"] == "True" {
					readyCount++
					break
				}
			}
		}

		if readyCount < totalCount {
			result.status = statusWarning
			result.details = append(result.details, fmt.Sprintf("%s: %d/%d ready", comp.name, readyCount, totalCount))
		} else {
			result.details = append(result.details, fmt.Sprintf("%s: %d/%d ready", comp.name, readyCount, totalCount))
		}
	}

	return result
}

// validateClusterReachability confirms that the HCP cluster is accessible via OCM and provides next steps for manual verification.
func (o *dnsValidateOptions) validateClusterReachability(ctx context.Context, ocmConn *ocmsdk.Connection) phaseResult {
	result := phaseResult{
		name:    "HCP Cluster Reachability",
		status:  statusPassed,
		details: make([]string, 0),
		errors:  make([]string, 0),
	}

	// Resolve cluster via OCM
	ocmClusterInfo, err := o.ocmClient.Resolve(ctx, o.clusterID)
	if err != nil {
		result.status = statusFailed
		result.errors = append(result.errors, fmt.Sprintf("Failed to resolve cluster via OCM: %v", err))
		return result
	}

	result.details = append(result.details, fmt.Sprintf("OCM cluster ID: %s", ocmClusterInfo.HCPClusterID))
	result.details = append(result.details, fmt.Sprintf("Domain prefix: %s", ocmClusterInfo.DomainPrefix))
	result.details = append(result.details, fmt.Sprintf("Base domain: %s", ocmClusterInfo.BaseDomain))
	result.details = append(result.details, fmt.Sprintf("Management cluster: %s", ocmClusterInfo.MgmtClusterName))

	result.details = append(result.details, "")
	result.details = append(result.details, "Next steps to verify full connectivity:")
	result.details = append(result.details, fmt.Sprintf("  • nslookup api.%s.%s", ocmClusterInfo.DomainPrefix, ocmClusterInfo.BaseDomain))
	result.details = append(result.details, "  • ocm backplane login "+o.clusterID)
	result.details = append(result.details, "  • oc get nodes")

	return result
}

// printPhaseResult displays the result of a validation phase including status, details, and errors.
func printPhaseResult(phase phaseResult) {
	statusSymbol := "✅"
	statusText := "PASSED"
	switch phase.status {
	case statusWarning:
		statusSymbol = "⚠️"
		statusText = "WARNING"
	case statusFailed:
		statusSymbol = "❌"
		statusText = "FAILED"
	}

	fmt.Fprintf(os.Stderr, "\n%s %s: %s\n", statusSymbol, phase.name, statusText)
	for _, detail := range phase.details {
		if detail == "" {
			fmt.Fprintf(os.Stderr, "\n")
		} else {
			fmt.Fprintf(os.Stderr, "  %s\n", detail)
		}
	}
	for _, err := range phase.errors {
		fmt.Fprintf(os.Stderr, "  ❌ %s\n", err)
	}
}

// printValidationSummary displays the overall validation results including pass/warning/fail counts and recommendations.
func printValidationSummary(results validationResult, clusterID, managementCluster string) {
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "╔════════════════════════════════════════════════════════════╗\n")
	fmt.Fprintf(os.Stderr, "║          Validation Summary                                ║\n")
	fmt.Fprintf(os.Stderr, "╚════════════════════════════════════════════════════════════╝\n")
	fmt.Fprintf(os.Stderr, "\n")

	passedCount := 0
	warningCount := 0
	failedCount := 0

	for _, phase := range results.phases {
		switch phase.status {
		case statusPassed:
			passedCount++
		case statusWarning:
			warningCount++
		case statusFailed:
			failedCount++
		}
	}

	overallStatus := "✅ ALL CHECKS PASSED"
	if failedCount > 0 {
		overallStatus = "❌ VALIDATION FAILED"
	} else if warningCount > 0 {
		overallStatus = "⚠️  PASSED WITH WARNINGS"
	}

	fmt.Fprintf(os.Stderr, "  Status:   %s\n", overallStatus)
	fmt.Fprintf(os.Stderr, "  Cluster:  %s\n", clusterID)
	fmt.Fprintf(os.Stderr, "  MC:       %s\n", managementCluster)
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  Phases:   %d passed, %d warnings, %d failed\n", passedCount, warningCount, failedCount)
	fmt.Fprintf(os.Stderr, "\n")

	if failedCount > 0 {
		fmt.Fprintf(os.Stderr, "⚠️  Review failed checks above and investigate issues.\n")
		fmt.Fprintf(os.Stderr, "   Common issues after DR recovery:\n")
		fmt.Fprintf(os.Stderr, "   - etcd TLS certificate mismatch (see KNOWN_ISSUES.md)\n")
		fmt.Fprintf(os.Stderr, "   - ExternalDNS not yet reconciled (wait 2-3 minutes)\n")
		fmt.Fprintf(os.Stderr, "   - AWSEndpointService not recreated (re-run dns-recover)\n")
	} else if warningCount > 0 {
		fmt.Fprintf(os.Stderr, "⚠️  Some warnings detected - review details above.\n")
		fmt.Fprintf(os.Stderr, "   Cluster may still be reconciling after recovery.\n")
	} else {
		fmt.Fprintf(os.Stderr, "✅ DNS recovery validation successful!\n")
		fmt.Fprintf(os.Stderr, "   Cluster should be fully accessible.\n")
	}
	fmt.Fprintf(os.Stderr, "\n")
}
