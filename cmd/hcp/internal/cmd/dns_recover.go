package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/openshift-online/ocm-sdk-go"
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DNSRecoveryRequest struct {
	ClusterID                string
	ManagementCluster        string
	SkipDNSVerification      bool
	DryRun                   bool
	ElevateReason            string
	RestartNetworkComponents bool
}

type DNSRecoveryPlan struct {
	ClusterID                string
	APIRouterStatus          string
	AWSEndpointServiceStatus string
	ExistingResources        []string
	RestartNetworkComponents bool

	// Internal state
	clusterInfo         *clusterInfo
	k8sClient           internalclient.ManagementClusterClient
	oldPrivateRouterUID string
}

type DNSRecoveryResult struct {
	APIRouterRecreated          bool
	AWSEndpointServiceRecreated bool
	NewResources                []string
	ComponentRestartSucceeded   bool
	ComponentRestartErrors      []string
}

type clusterInfo struct {
	ClusterID         string
	HCPNamespace      string
	HostedClusterNS   string
	DomainPrefix      string
	ManagementCluster string
}

type DNSRecoveryManager interface {
	PrepareRecovery(ctx context.Context, req DNSRecoveryRequest) (DNSRecoveryPlan, error)
	RecoverDNS(ctx context.Context, plan DNSRecoveryPlan) (DNSRecoveryResult, error)
}

type DefaultDNSRecoveryManager struct {
	ocmClient internalclient.OCMClient
	awsClient internalclient.AWSAccountClientProvider
	logger    *logrus.Logger
}

func NewDefaultDNSRecoveryManager(ocmClient internalclient.OCMClient, logger *logrus.Logger) *DefaultDNSRecoveryManager {
	return &DefaultDNSRecoveryManager{
		ocmClient: ocmClient,
		awsClient: internalclient.NewAWSAccountClientProvider(),
		logger:    logger,
	}
}

func (m *DefaultDNSRecoveryManager) PrepareRecovery(ctx context.Context, req DNSRecoveryRequest) (DNSRecoveryPlan, error) {
	plan := DNSRecoveryPlan{
		ClusterID:                req.ClusterID,
		RestartNetworkComponents: req.RestartNetworkComponents,
	}

	m.logger.Infof("Preparing DNS recovery for cluster %s on management cluster %s", req.ClusterID, req.ManagementCluster)

	// Resolve cluster info from OCM
	ocmClusterInfo, err := m.ocmClient.Resolve(ctx, req.ClusterID)
	if err != nil {
		return plan, fmt.Errorf("resolving cluster: %w", err)
	}

	hcpNamespace, err := m.ocmClient.HCPNamespace(ctx, ocmClusterInfo.HCPClusterID)
	if err != nil {
		return plan, fmt.Errorf("resolving HCP namespace: %w", err)
	}

	// Build cluster info struct for recovery
	info := &clusterInfo{
		ClusterID:         req.ClusterID,
		ManagementCluster: req.ManagementCluster,
		DomainPrefix:      ocmClusterInfo.DomainPrefix,
		HCPNamespace:      hcpNamespace,
		HostedClusterNS:   strings.TrimSuffix(hcpNamespace, "-"+ocmClusterInfo.DomainPrefix),
	}

	if info.HCPNamespace == "" || info.HostedClusterNS == "" {
		return plan, fmt.Errorf("cluster %s does not have valid namespace configuration", req.ClusterID)
	}

	plan.clusterInfo = info

	ocmConn, err := utils.CreateConnection()
	if err != nil {
		return plan, fmt.Errorf("creating OCM connection: %w", err)
	}

	defer ocmConn.Close()

	m.ocmClient = *internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: m.logger})

	plan.k8sClient, err = m.ocmClient.ElevatedManagementCluster(req.ManagementCluster, req.ElevateReason)
	if err != nil {
		return plan, fmt.Errorf("connecting to management cluster %s: %w", req.ManagementCluster, err)
	}

	// Verify DNS cleanup (if not skipping DNS verification)
	if !req.SkipDNSVerification {
		if err := m.verifyDNSCleanup(ctx, ocmConn, &plan); err != nil {
			return plan, err
		}
	}

	// Check etcd health before proceeding
	if err := m.checkEtcdHealth(ctx, &plan); err != nil {
		m.logger.Warnf("etcd health check: %v", err)
		m.logger.Warnf("DNS recovery may not fully resolve connectivity if etcd is unhealthy")
	}

	// Check current K8s resource state
	if err := m.checkResourceState(ctx, &plan); err != nil {
		return plan, err
	}

	m.logger.Infof("Recovery plan prepared: HCP namespace=%s, HC namespace=%s", info.HCPNamespace, info.HostedClusterNS)

	return plan, nil
}

func (m *DefaultDNSRecoveryManager) RecoverDNS(ctx context.Context, plan DNSRecoveryPlan) (DNSRecoveryResult, error) {
	result := DNSRecoveryResult{}

	m.logger.Infof("Starting DNS recovery...")

	// Delete both the LoadBalancer Service and AWSEndpointService to force controller rediscovery
	if plan.AWSEndpointServiceStatus != "missing" {
		m.logger.Infof("Deleting Service and AWSEndpointService private-router in namespace %s", plan.clusterInfo.HCPNamespace)

		// Delete the LoadBalancer Service
		if err := plan.k8sClient.DeleteService(ctx, plan.clusterInfo.HCPNamespace, "private-router"); err != nil {
			m.logger.Warnf("Failed to delete Service: %v", err)
		}

		// Delete the AWSEndpointService
		if err := plan.k8sClient.DeleteAWSEndpointService(ctx, plan.clusterInfo.HCPNamespace, "private-router"); err != nil {
			return result, fmt.Errorf("deleting AWSEndpointService private-router: %w", err)
		}
		result.AWSEndpointServiceRecreated = true
	}

	// Poll for resource recreation
	m.logger.Infof("Waiting for resources to be recreated...")
	if err := m.pollResourceRecreation(ctx, &plan, 30*time.Second); err != nil {
		return result, fmt.Errorf("polling for resource recreation: %w", err)
	}

	// Restart control plane components if requested
	if plan.RestartNetworkComponents {
		m.logger.Infof("Restarting control plane components to reconnect to new endpoints...")
		restartErrors := m.restartControlPlaneComponents(ctx, &plan)
		result.ComponentRestartErrors = restartErrors
		result.ComponentRestartSucceeded = len(restartErrors) == 0
	} else {
		m.logger.Infof("Skipping control plane restart (use --restart-network-components to enable)")
		result.ComponentRestartSucceeded = true // Not attempted, so not a failure
	}

	// Capture new resource state
	if err := m.captureNewResources(ctx, &plan, &result); err != nil {
		m.logger.Warnf("Failed to capture new resources: %v", err)
	}

	m.logger.Infof("DNS recovery completed successfully")

	return result, nil
}

func (m *DefaultDNSRecoveryManager) verifyDNSCleanup(ctx context.Context, ocmConn *sdk.Connection, plan *DNSRecoveryPlan) error {

	// Query Route 53 for existing DNS records in central DNS account
	m.logger.Infof("  Querying Route 53 for DNS records...")

	var releaser DNSReleaser = NewDefaultDNSReleaser(internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: m.logger}))
	releaserPlan, err := releaser.PrepareDNSRelease(ctx, ReleaseDNSRequest{ClusterID: plan.ClusterID})
	if err != nil {
		return fmt.Errorf("querying DNS records: %w", err)
	}

	m.logger.Infof("Verifying DNS cleanup in central DNS account (%s)", releaserPlan.AccountID)
	m.logger.Infof("  Cluster domain prefix: %s", plan.clusterInfo.DomainPrefix)
	m.logger.Infof("  Cluster ID: %s", plan.ClusterID)
	m.logger.Infof("  Hosted zone: %s (%s)", releaserPlan.ZoneName, releaserPlan.ZoneID)

	m.logger.Infof("  DNS Records Found:")
	if len(releaserPlan.Records) == 0 {
		m.logger.Infof("    • [empty - DNS cleanup already completed]")
		m.logger.Infof("Route 53 DNS cleanup verification complete")
		return nil
	}

	// DNS records exist - could be stale (cleanup not done) or new (already reconciled)
	m.logger.Warnf("    • Found %d DNS record(s):", len(releaserPlan.Records))
	for _, record := range releaserPlan.Records {
		m.logger.Warnf("      - %s (%s)", *record.Name, record.Type)
	}

	m.logger.Warnf("")
	m.logger.Warnf("These records may indicate either:")
	m.logger.Warnf("  1. DNS cleanup is incomplete (run 'osdctl hcp restore release-dns')")
	m.logger.Warnf("  2. ExternalDNS has already reconciled the new records (safe to proceed)")
	m.logger.Warnf("")

	return fmt.Errorf("DNS records exist - review above and either:\n\n"+
		"Option 1: Clean up stale records first:\n"+
		"  osdctl hcp restore release-dns --cluster-id %s\n\n"+
		"Option 2: Skip DNS verification if records are already reconciled:\n"+
		"  osdctl hcp recover --cluster-id %s --management-cluster %s --elevate-reason \"<reason>\" --skip-dns-verification",
		plan.ClusterID, plan.ClusterID, plan.clusterInfo.ManagementCluster)
}

func (m *DefaultDNSRecoveryManager) checkEtcdHealth(ctx context.Context, plan *DNSRecoveryPlan) error {
	m.logger.Infof("Checking etcd health in namespace %s", plan.clusterInfo.HCPNamespace)

	kubeClient := plan.k8sClient.GetRuntimeClient()

	// Check etcd StatefulSet
	etcdSts := &unstructured.Unstructured{}
	etcdSts.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"})
	key := client.ObjectKey{Namespace: plan.clusterInfo.HCPNamespace, Name: "etcd"}
	if err := kubeClient.Get(ctx, key, etcdSts); err != nil {
		return fmt.Errorf("etcd StatefulSet not found: %w", err)
	}

	replicas, _, _ := unstructured.NestedInt64(etcdSts.Object, "spec", "replicas")
	readyReplicas, _, _ := unstructured.NestedInt64(etcdSts.Object, "status", "readyReplicas")

	m.logger.Infof("etcd: %d/%d ready", readyReplicas, replicas)

	if readyReplicas < replicas {
		return fmt.Errorf("etcd not fully ready (%d/%d) - check etcd pod logs for TLS certificate errors. "+
			"After cross-MC migration, etcd peer TLS certs may need regeneration: "+
			"delete secrets etcd-peer-tls, etcd-server-tls, etcd-client-tls, etcd-signer "+
			"and restart control-plane-operator", readyReplicas, replicas)
	}

	return nil
}

func (m *DefaultDNSRecoveryManager) restartControlPlaneComponents(ctx context.Context, plan *DNSRecoveryPlan) []string {
	kubeClient := plan.k8sClient.GetRuntimeClient()
	ns := plan.clusterInfo.HCPNamespace
	var errors []string

	componentsToRestart := []struct {
		label  string
		reason string
	}{
		{"app=kube-apiserver", "reconnect to new VPC endpoint and etcd"},
		{"app=konnectivity-agent", "re-establish konnectivity tunnel"},
		{"app=control-plane-operator", "trigger reconciliation of control plane"},
		{"app=hosted-cluster-config-operator", "reconcile hosted cluster configuration"},
		{"app=ovnkube-control-plane", "reconnect OVN to new management cluster"},
		{"app=private-router", "reconnect router to new endpoints"},
	}

	for _, comp := range componentsToRestart {
		m.logger.Infof("Restarting pods with label %s (%s)", comp.label, comp.reason)

		podList := &unstructured.UnstructuredList{}
		podList.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PodList"})

		labelKey := strings.SplitN(comp.label, "=", 2)[0]
		labelVal := strings.SplitN(comp.label, "=", 2)[1]

		if err := kubeClient.List(ctx, podList, client.InNamespace(ns), client.MatchingLabels{labelKey: labelVal}); err != nil {
			errMsg := fmt.Sprintf("Failed to list pods with label %s: %v", comp.label, err)
			m.logger.Warnf("  %s", errMsg)
			errors = append(errors, errMsg)
			continue
		}

		deleted := 0
		deleteFailed := 0
		for i := range podList.Items {
			pod := &podList.Items[i]
			if err := kubeClient.Delete(ctx, pod); err != nil {
				if !apierrors.IsNotFound(err) {
					errMsg := fmt.Sprintf("Failed to delete pod %s: %v", pod.GetName(), err)
					m.logger.Warnf("  %s", errMsg)
					errors = append(errors, errMsg)
					deleteFailed++
				}
			} else {
				deleted++
			}
		}
		if deleted > 0 {
			m.logger.Infof("  Deleted %d pod(s)", deleted)
		} else {
			m.logger.Debugf("  No pods found with label %s", comp.label)
		}
	}

	m.logger.Infof("Waiting 30s for control plane components to restart...")
	time.Sleep(30 * time.Second)

	return errors
}

func (m *DefaultDNSRecoveryManager) checkResourceState(ctx context.Context, plan *DNSRecoveryPlan) error {
	m.logger.Infof("Checking current resource state in namespace %s", plan.clusterInfo.HCPNamespace)

	// Check private-router AWSEndpointService via the management cluster client method
	endpoint, err := plan.k8sClient.GetAWSEndpointService(ctx, plan.clusterInfo.HCPNamespace, "private-router")

	switch {
	case err == nil && endpoint.GetDeletionTimestamp() == nil:
		plan.AWSEndpointServiceStatus = "exists"
		plan.oldPrivateRouterUID = string(endpoint.GetUID())
		m.logger.Debugf("Captured existing private-router UID: %s", plan.oldPrivateRouterUID)
	case apierrors.IsNotFound(err):
		plan.AWSEndpointServiceStatus = "missing"
	default:
		return fmt.Errorf("checking private-router AWSEndpointService: %w", err)
	}

	plan.APIRouterStatus = "missing"

	m.logger.Infof("Resource state: private-router=%s",
		plan.AWSEndpointServiceStatus)

	return nil
}

func (m *DefaultDNSRecoveryManager) pollResourceRecreation(ctx context.Context, plan *DNSRecoveryPlan, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	m.logger.Infof("Polling for resource recreation with %d second timeout", int(timeout.Seconds()))

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		obj, err := plan.k8sClient.GetAWSEndpointService(ctx, plan.clusterInfo.HCPNamespace, "private-router")
		if err == nil && obj.GetDeletionTimestamp() == nil && obj.GetUID() != types.UID(plan.oldPrivateRouterUID) {
			m.logger.Infof("Resource private-router recreated successfully")
			return nil
		}

		m.logger.Debugf("Waiting for private-router recreation...")
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("timed out after %d seconds waiting for private-router AWSEndpointService recreation", int(timeout.Seconds()))
}

func (m *DefaultDNSRecoveryManager) captureNewResources(ctx context.Context, plan *DNSRecoveryPlan, result *DNSRecoveryResult) error {
	// Get AWSEndpointService details
	_, err := plan.k8sClient.GetAWSEndpointService(ctx, plan.clusterInfo.HCPNamespace, "private-router")
	if err == nil {
		result.NewResources = append(result.NewResources, "awsendpointservice/private-router")
	}

	return nil
}

// FormatRecoveryResults returns a formatted string representation of recovery results
func FormatRecoveryResults(result DNSRecoveryResult, plan DNSRecoveryPlan, clusterID string, managementCluster string, dryRun bool) string {
	output := "\n"
	output += "╔════════════════════════════════════════════════════════════╗\n"
	output += "║          DNS Recovery Results                              ║\n"
	output += "╚════════════════════════════════════════════════════════════╝\n"
	output += "\n"

	if dryRun {
		output += "Status:   DRY-RUN (No changes made)\n"
		output += fmt.Sprintf("Cluster:  %s\n", clusterID)
		output += fmt.Sprintf("MC:       %s\n", managementCluster)
		output += "\n"
		output += "Preview of actions that would be taken:\n"

		if plan.AWSEndpointServiceStatus == "exists" {
			output += "  ✓ Delete private-router Service and AWSEndpointService\n"
			output += "  ✓ Poll for resource recreation (30s timeout)\n"
		} else {
			output += "  ⊘ private-router not found - skip deletion\n"
			output += "  ✓ Poll for resource creation (30s timeout)\n"
		}

		if plan.RestartNetworkComponents {
			output += "  ✓ Restart control plane components:\n"
			output += "      - kube-apiserver (reconnect to new VPC endpoint)\n"
			output += "      - konnectivity-agent (re-establish tunnel)\n"
			output += "      - control-plane-operator (trigger reconciliation)\n"
			output += "      - hosted-cluster-config-operator (reconcile config)\n"
			output += "      - ovnkube-control-plane (reconnect OVN)\n"
			output += "      - private-router (reconnect to new endpoints)\n"
		} else {
			output += "  ⊘ Skip control plane restart (--restart-network-components not set)\n"
		}

		output += "  ✓ Capture new resource state\n"
		return output
	}

	status := "✅ SUCCESS"
	if !result.APIRouterRecreated && !result.AWSEndpointServiceRecreated {
		status = "⚠️  PARTIAL - No resources needed recovery"
	} else if plan.RestartNetworkComponents && !result.ComponentRestartSucceeded {
		status = "⚠️  PARTIAL - Component restart failures occurred"
	}

	output += fmt.Sprintf("Status:   %s\n", status)
	output += fmt.Sprintf("Cluster:  %s\n", clusterID)
	output += fmt.Sprintf("MC:       %s\n", managementCluster)
	output += "\n"

	output += "Recovered Resources:\n"
	if result.AWSEndpointServiceRecreated {
		output += "  ✓ private-router (AWSEndpointSvc) [DELETED & RECREATED]\n"
	} else {
		output += "  ⊘ private-router                  [NOT FOUND]\n"
	}

	output += "\n"

	if len(result.NewResources) > 0 {
		output += "New/Updated Resources:\n"
		for _, res := range result.NewResources {
			output += fmt.Sprintf("  → %s\n", res)
		}
		output += "\n"
	}

	output += "Recovery Actions:\n"
	output += "  1. ✓ Checked etcd health\n"
	output += "  2. ✓ Checked private-router AWSEndpointService state\n"

	if result.AWSEndpointServiceRecreated {
		output += "  3. ✓ Deleted private-router Service and AWSEndpointService\n"
		output += "  4. ✓ Polled for resource recreation (30s timeout)\n"
	} else {
		output += "  3. ⊘ Skipped deletion (resource not found)\n"
		output += "  4. ✓ Polled for resource creation (30s timeout)\n"
	}

	if plan.RestartNetworkComponents {
		if result.ComponentRestartSucceeded {
			output += "  5. ✓ Restarted control plane components\n"
			output += "        (kube-apiserver, konnectivity-agent, CPO, HCCO, OVN, router)\n"
		} else {
			output += "  5. ⚠️  Partial component restart (some failures occurred)\n"
			output += "        (kube-apiserver, konnectivity-agent, CPO, HCCO, OVN, router)\n"
		}
	} else {
		output += "  5. ⊘ Skipped control plane restart (--restart-network-components not set)\n"
	}

	output += "  6. ✓ Captured new resource state\n"
	output += "\n"

	if len(result.ComponentRestartErrors) > 0 {
		output += "Component Restart Errors:\n"
		for _, err := range result.ComponentRestartErrors {
			output += fmt.Sprintf("  ⚠️  %s\n", err)
		}
		output += "\n"
	}

	output += "Next Steps:\n"
	output += "  • Wait 2-3 min for ExternalDNS to reconcile Route 53 records\n"
	output += "  • Verify: nslookup api.<domain-prefix>.<base-domain>\n"
	output += "  • Verify: ocm backplane login <cluster-id>\n"
	output += "  • If API unreachable, check etcd logs for TLS cert errors\n"
	output += "  • Review control-plane-operator and ExternalDNS logs if issues persist\n"
	output += "\n"

	output += "╔════════════════════════════════════════════════════════════╗\n"
	output += fmt.Sprintf("║ Recovery completed at %s\n", time.Now().Format("2006-01-02 15:04:05 MST"))
	output += "╚════════════════════════════════════════════════════════════╝\n"

	return output
}
