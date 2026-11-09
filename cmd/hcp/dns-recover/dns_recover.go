package dnsrecover

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type dnsRecoverOptions struct {
	clusterID                string
	managementCluster        string
	dryRun                   bool
	skipDNSVerification      bool
	reason                   string
	debug                    bool
	restartNetworkComponents bool

	// Populated during validation
	ocmClient *internalclient.OCMClient
	logger    *logrus.Logger
}

func newCmdDNSRecover() *cobra.Command {
	opts := &dnsRecoverOptions{}
	cmd := &cobra.Command{
		Use:   "recover --cluster-id <hcp-id> --management-cluster <mc-name> --reason <reason>",
		Short: "Recover ExternalDNS and AWSEndpointService reconciliation for a restored HCP cluster",
		Long: `Recover ExternalDNS and AWSEndpointService reconciliation after HCP management cluster disaster recovery.

When a hosted cluster is restored on a replacement management cluster, the AWSEndpointService
controller may fail to reconcile the existing load balancer endpoint. This command
automates recovery by:

  1. Verifying DNS cleanup (old Route 53 TXT ownership records removed)
  2. Confirming cluster resource state (private-router AWSEndpointService)
  3. Deleting private-router to force controller rediscovery
  4. Polling for resource recreation with automatic reconciliation
  5. Outputting new/updated resources after reconciliation

The command supports --dry-run to preview changes without making modifications, and
--skip-dns-verification to resume after DNS cleanup has been verified separately.`,
		Example: `  # Preview recovery without making changes
  osdctl hcp recover --cluster-id my-hcp-cluster --management-cluster dest-mgmt-cluster --reason "DR recovery"

  # Preview with all flags
  osdctl hcp recover --cluster-id my-hcp-cluster --management-cluster dest-mgmt-cluster --reason "DR recovery" --dry-run

  # Resume recovery after DNS cleanup
  osdctl hcp recover --cluster-id my-hcp-cluster --management-cluster dest-mgmt-cluster --reason "DR recovery" --skip-dns-verification`,
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
	cmd.Flags().StringVarP(&opts.managementCluster, "management-cluster", "M", "", "Name of the replacement management cluster (required)")
	cmd.Flags().StringVar(&opts.reason, "reason", "", "Reason for elevated access to management cluster")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Preview changes without deleting resources")
	cmd.Flags().BoolVar(&opts.skipDNSVerification, "skip-dns-verification", false, "Skip Route 53 TXT ownership record verification (use if DNS cleanup already verified)")
	cmd.Flags().BoolVar(&opts.restartNetworkComponents, "restart-network-components", false, "Restart kube-apiserver, konnectivity, CPO, HCCO, OVN, and router after recovery")
	cmd.Flags().BoolVar(&opts.debug, "debug", false, "Enable debug logging output")

	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("management-cluster")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}

func (o *dnsRecoverOptions) validate(ctx context.Context, ocmConn *ocmsdk.Connection) error {
	if o.clusterID == "" || o.managementCluster == "" || o.reason == "" {
		return fmt.Errorf("--cluster-id, --management-cluster, and --reason are required")
	}

	if ocmConn != nil {
		o.ocmClient = internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: o.logger})
	}

	return nil
}

func (o *dnsRecoverOptions) Run(ctx context.Context) error {
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

	recoverymanager := internalcmd.NewDefaultDNSRecoveryManager(*o.ocmClient, o.logger)
	req := internalcmd.DNSRecoveryRequest{
		ClusterID:                o.clusterID,
		ManagementCluster:        o.managementCluster,
		SkipDNSVerification:      o.skipDNSVerification,
		DryRun:                   o.dryRun,
		ElevateReason:            o.reason,
		RestartNetworkComponents: o.restartNetworkComponents,
	}

	// Phase 1: Plan - gather cluster state and build recovery plan
	o.logger.Infof("Phase 1/3: Building recovery plan...")
	plan, err := recoverymanager.PrepareRecovery(ctx, req)
	if err != nil {
		// Check if error is due to existing DNS records
		var dnsErr *internalcmd.DNSRecordsExistError
		if errors.As(err, &dnsErr) {
			// Prompt user to continue despite DNS records
			fmt.Fprintf(os.Stderr, "\n")
			if !askDNSConfirmation() {
				fmt.Fprintf(os.Stderr, "\nRecovery cancelled. To clean up DNS records first, run:\n")
				fmt.Fprintf(os.Stderr, "  osdctl hcp restore release-dns --cluster-id %s\n", o.clusterID)
				return nil
			}
			// User confirmed - recreate client and retry with DNS verification skipped
			o.ocmClient = internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: o.logger})
			recoverymanager = internalcmd.NewDefaultDNSRecoveryManager(*o.ocmClient, o.logger)
			req.SkipDNSVerification = true
			plan, err = recoverymanager.PrepareRecovery(ctx, req)
			if err != nil {
				return fmt.Errorf("unable to complete planned recovery for hcp cluster %s to mgmt cluster %s : %w", o.clusterID, o.managementCluster, err)
			}
		} else {
			return fmt.Errorf("unable to complete planned recovery for hcp cluster %s to mgmt cluster %s : %w", o.clusterID, o.managementCluster, err)
		}
	}

	// Phase 2: Verify - show plan and get user confirmation
	o.logger.Infof("Phase 2/3: Recovery plan ready")
	printRecoveryPlan(plan, o.clusterID, o.managementCluster)

	if o.dryRun {
		fmt.Fprintf(os.Stderr, "\n--dry-run specified, no changes will be made.\n")
		output := internalcmd.FormatRecoveryResults(internalcmd.DNSRecoveryResult{}, plan, o.clusterID, o.managementCluster, true)
		fmt.Fprint(os.Stdout, output)
		return nil
	}

	if !askConfirmation(o.logger) {
		return nil
	}

	// Phase 3: Execute - perform recovery actions
	o.logger.Infof("Phase 3/3: Executing recovery...")
	result, err := recoverymanager.RecoverDNS(ctx, plan)
	if err != nil {
		return fmt.Errorf("recovery failed: %w", err)
	}

	// Output formatted results
	output := internalcmd.FormatRecoveryResults(result, plan, o.clusterID, o.managementCluster, false)
	fmt.Fprint(os.Stdout, output)

	return nil
}

func printRecoveryPlan(plan internalcmd.DNSRecoveryPlan, clusterID, managementCluster string) {
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "╔════════════════════════════════════════════════════════════╗\n")
	fmt.Fprintf(os.Stderr, "║          DNS Recovery Plan                                 ║\n")
	fmt.Fprintf(os.Stderr, "╚════════════════════════════════════════════════════════════╝\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  Cluster:  %s\n", clusterID)
	fmt.Fprintf(os.Stderr, "  MC:       %s\n", managementCluster)
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  Current State:\n")
	fmt.Fprintf(os.Stderr, "    private-router (AWSEndpointService): %s\n", plan.AWSEndpointServiceStatus)
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  Actions to be taken:\n")

	if plan.AWSEndpointServiceStatus == "exists" {
		fmt.Fprintf(os.Stderr, "    1. Delete private-router Service (LoadBalancer)\n")
		fmt.Fprintf(os.Stderr, "    2. Delete private-router AWSEndpointService\n")
		fmt.Fprintf(os.Stderr, "    3. Wait for controller to recreate resources (30s timeout)\n")
	} else {
		fmt.Fprintf(os.Stderr, "    [private-router not found - no deletion needed]\n")
		fmt.Fprintf(os.Stderr, "    1. Wait for controller to create resources (30s timeout)\n")
	}

	if plan.RestartNetworkComponents {
		fmt.Fprintf(os.Stderr, "    4. Restart control plane components:\n")
		fmt.Fprintf(os.Stderr, "       - kube-apiserver\n")
		fmt.Fprintf(os.Stderr, "       - konnectivity-agent\n")
		fmt.Fprintf(os.Stderr, "       - control-plane-operator\n")
		fmt.Fprintf(os.Stderr, "       - hosted-cluster-config-operator\n")
		fmt.Fprintf(os.Stderr, "       - ovnkube-control-plane\n")
		fmt.Fprintf(os.Stderr, "       - private-router\n")
	} else {
		fmt.Fprintf(os.Stderr, "    4. [SKIP] Restart control plane (use --restart-network-components)\n")
	}
	fmt.Fprintf(os.Stderr, "    5. Capture new resource state\n")
	fmt.Fprintf(os.Stderr, "\n")
}

func askDNSConfirmation() bool {
	fmt.Fprintf(os.Stderr, "DNS records were found for this cluster. These could be:\n")
	fmt.Fprintf(os.Stderr, "  1. NEW records (already reconciled by ExternalDNS) - SAFE to proceed\n")
	fmt.Fprintf(os.Stderr, "  2. OLD records (stale, not cleaned up) - should run 'release-dns' first\n")
	fmt.Fprintf(os.Stderr, "\nIf you already ran 'release-dns' and records were recreated, it's safe to proceed.\n")
	fmt.Fprintf(os.Stderr, "\nProceed with recovery despite DNS records? (y/N): ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nRecovery cancelled (error reading response)\n")
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

func askConfirmation(logger *logrus.Logger) bool {
	// Write to stderr so stdout can be piped/redirected for other uses
	fmt.Fprintf(os.Stderr, "\n⚠️  DESTRUCTIVE ACTION: This will delete the following resource:\n")
	fmt.Fprintf(os.Stderr, "  • AWSEndpointService (private-router in HCP namespace)\n")
	fmt.Fprintf(os.Stderr, "\nThe resource will be automatically recreated and reconciled with existing AWS\n")
	fmt.Fprintf(os.Stderr, "resources. No AWS resources will be deleted.\n")
	fmt.Fprintf(os.Stderr, "\n⚠️  This action cannot be undone.\n")
	fmt.Fprintf(os.Stderr, "Do you want to proceed? (y/N): ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		logger.Errorf("error reading user confirmation: %v", err)
		fmt.Fprintf(os.Stderr, "\nRecovery cancelled (error reading response)\n")
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		logger.Infof("recovery cancelled by user")
		fmt.Fprintf(os.Stderr, "Recovery cancelled by user\n")
		return false
	}

	logger.Infof("user confirmed recovery operation")
	fmt.Fprintf(os.Stderr, "Proceeding with recovery...\n\n")
	return true
}
