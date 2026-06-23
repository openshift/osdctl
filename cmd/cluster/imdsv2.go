package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	awshivev1 "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/osdctl/pkg/infra"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ControlPlaneMachineSet location
	cpmsNamespace = "openshift-machine-api"
	cpmsName      = "cluster"

	// Timeouts and intervals
	imdsv2PollInterval       = 30 * time.Second
	imdsv2RolloutPollTimeout = 120 * time.Minute
	imdsv2MachineWaitTimeout = 15 * time.Minute
	imdsv2NodeWaitTimeout    = 15 * time.Minute
	imdsv2COWaitTimeout      = 15 * time.Minute

	// IMDS authentication modes
	imdsv2Required = "Required"
	imdsv2Optional = "Optional"

	// Hive MachinePool override annotation
	hiveOverrideAnnotation = "hive.openshift.io/override-machinepool-platform"
)

type imdsv2Options struct {
	clusterID string
	cluster   *cmv1.Cluster
	reason    string
	nodeRoles string // "all" (default), "master", "infra", "workers"

	client          client.Client
	clientAdmin     client.Client
	hiveClient      client.Client
	hiveAdminClient client.Client
}

func newCmdIMDSv2() *cobra.Command {
	ops := &imdsv2Options{}
	cmd := &cobra.Command{
		Use:   "imdsv2",
		Short: "Migrate cluster nodes to enforce IMDSv2 (Instance Metadata Service v2)",
		Long: `Migrate ROSA Classic cluster nodes to enforce IMDSv2.

This automates the SOP for migrating machines to IMDSv2 by:
- Patching Hive MachinePools to require IMDSv2
- Replacing infra nodes (one at a time)
- Updating ControlPlaneMachineSet for automatic master node rollout
- Validating all nodes/machines are using IMDSv2

Pre-flight checks verify cluster health before making changes.`,
		Example: `  # Migrate all nodes (infra + masters)
  osdctl cluster imdsv2 -C ${CLUSTER_ID} --reason "JIRA-12345"

  # Migrate only infra nodes
  osdctl cluster imdsv2 -C ${CLUSTER_ID} --reason "CASE-67890" --nodes infra

  # Migrate only master nodes
  osdctl cluster imdsv2 -C ${CLUSTER_ID} --reason "JIRA-12345" --nodes master`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		SilenceUsage:      true, // Don't show usage on errors
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.run(context.Background())
		},
	}

	cmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "The internal/external ID of the cluster")
	cmd.Flags().StringVar(&ops.reason, "reason", "", "Reason for elevation (OHSS/PD/JIRA ticket)")
	cmd.Flags().StringVar(&ops.nodeRoles, "nodes", "all", "Node roles to migrate: all, master, infra, workers")

	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}

func (o *imdsv2Options) validate() error {
	if err := utils.IsValidClusterKey(o.clusterID); err != nil {
		return err
	}

	validRoles := map[string]bool{"all": true, "master": true, "infra": true, "workers": true}
	if !validRoles[o.nodeRoles] {
		return fmt.Errorf("invalid nodes: %s (must be 'all', 'master', 'infra', or 'workers')", o.nodeRoles)
	}

	return nil
}

func (o *imdsv2Options) init() error {
	connection, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer connection.Close()

	cluster, err := utils.GetCluster(connection, o.clusterID)
	if err != nil {
		return err
	}
	o.cluster = cluster
	o.clusterID = cluster.ID()

	// Validate cluster is AWS Classic (not GCP/Azure or HCP)
	if err := ValidateAWSClassicCluster(cluster); err != nil {
		return err
	}

	// Set up standard cluster clients
	clients, err := SetupClusterClients(o.clusterID, o.reason, "Migrating to IMDSv2")
	if err != nil {
		return err
	}
	o.client = clients.Client
	o.clientAdmin = clients.ClientAdmin

	// Set up Hive clients for infra/worker node replacement via MachinePools
	if o.nodeRoles == "all" || o.nodeRoles == "infra" || o.nodeRoles == "workers" {
		hc, hac, err := SetupHiveClients(o.clusterID, o.reason, "Migrating to IMDSv2")
		if err != nil {
			return err
		}
		o.hiveClient = hc
		o.hiveAdminClient = hac
	}

	return nil
}

func (o *imdsv2Options) run(ctx context.Context) error {
	// Validate command-line arguments
	if err := o.validate(); err != nil {
		return err
	}

	// Initialize OCM connection and Kubernetes clients
	if err := o.init(); err != nil {
		return err
	}

	fmt.Printf("Node Roles: %s\n\n", o.nodeRoles)

	// Verify cluster health before making changes
	if err := o.preFlightChecks(ctx); err != nil {
		return fmt.Errorf("pre-flight checks failed: %v", err)
	}

	// Determine which node types to migrate based on --nodes flag
	doInfra := o.nodeRoles == "all" || o.nodeRoles == "infra"
	doMasters := o.nodeRoles == "all" || o.nodeRoles == "master"
	doWorkers := o.nodeRoles == "all" || o.nodeRoles == "workers"

	// Track what actually changed
	var infraChanged bool
	var cpmsChanged bool
	var workersChanged bool

	// Step 1: Replace infra machines using MachinePool dance
	if doInfra {
		changed, err := o.migrateInfraToIMDSv2(ctx)
		if err != nil {
			return fmt.Errorf("infra migration failed: %v", err)
		}
		infraChanged = changed
	}

	// Step 2: Update ControlPlaneMachineSet to trigger master node rollout
	if doMasters {
		changed, err := o.updateCPMSForIMDSv2(ctx)
		if err != nil {
			return fmt.Errorf("CPMS update failed: %v", err)
		}
		cpmsChanged = changed
	}

	// Step 3: List and patch worker MachinePools (if requested)
	if doWorkers {
		changed, err := o.migrateWorkersToIMDSv2(ctx)
		if err != nil {
			return fmt.Errorf("worker migration failed: %v", err)
		}
		workersChanged = changed
	}

	// Step 4: Verify all nodes and machines are configured correctly
	if err := o.validateIMDSv2(ctx); err != nil {
		return fmt.Errorf("validation failed: %v", err)
	}

	// Only show success if we actually made changes
	if infraChanged || cpmsChanged || workersChanged {
		printer.PrintlnGreen("\n✓ IMDSv2 migration completed successfully!")
		if infraChanged {
			fmt.Println("  - Infra nodes migrated to IMDSv2")
		}
		if cpmsChanged {
			fmt.Println("  - Master nodes migrated to IMDSv2")
		}
		if workersChanged {
			fmt.Println("  - Worker nodes migrated to IMDSv2")
		}
	} else {
		printer.PrintlnGreen("\n✓ All components already configured for IMDSv2!")
	}
	return nil
}

// preFlightChecks verifies cluster health before making changes.
func (o *imdsv2Options) preFlightChecks(ctx context.Context) error {
	fmt.Println("Running pre-flight checks...")

	// Verify all ClusterOperators are Available and not Degraded
	// (This implicitly verifies etcd health via the etcd operator)
	if err := CheckClusterOperators(ctx, o.client); err != nil {
		return err
	}

	// Verify all 3 master nodes are Ready
	masterNodes := &corev1.NodeList{}
	if err := o.client.List(ctx, masterNodes, client.MatchingLabels{"node-role.kubernetes.io/master": ""}); err != nil {
		return fmt.Errorf("failed to list master nodes: %v", err)
	}
	readyMasters := CountReadyNodes(masterNodes)
	if readyMasters != 3 {
		return fmt.Errorf("expected 3 ready master nodes, found %d", readyMasters)
	}
	fmt.Printf("  Master nodes: %d/3 Ready\n", readyMasters)

	// Verify all infra nodes are Ready (if migrating infra)
	if o.nodeRoles == "all" || o.nodeRoles == "infra" {
		infraNodes := &corev1.NodeList{}
		if err := o.client.List(ctx, infraNodes, client.MatchingLabels{"node-role.kubernetes.io/infra": ""}); err != nil {
			return fmt.Errorf("failed to list infra nodes: %v", err)
		}
		readyInfra := CountReadyNodes(infraNodes)
		totalInfra := len(infraNodes.Items)
		if totalInfra == 0 {
			return fmt.Errorf("no infra nodes found")
		}
		if readyInfra != totalInfra {
			return fmt.Errorf("not all infra nodes are ready (%d/%d)", readyInfra, totalInfra)
		}
		fmt.Printf("  Infra nodes: %d/%d Ready\n", readyInfra, totalInfra)
	}

	// Verify CPMS is Active (only needed if migrating masters)
	if o.nodeRoles == "all" || o.nodeRoles == "master" {
		cpms := &machinev1.ControlPlaneMachineSet{}
		if err := o.client.Get(ctx, client.ObjectKey{Namespace: cpmsNamespace, Name: cpmsName}, cpms); err != nil {
			return fmt.Errorf("failed to get CPMS: %v", err)
		}
		if cpms.Spec.State != machinev1.ControlPlaneMachineSetStateActive {
			return fmt.Errorf("CPMS is not Active (state: %s). Cannot proceed with control plane changes", cpms.Spec.State)
		}
		// Don't print CPMS status - master nodes Ready is sufficient
	}

	printer.PrintlnGreen("  All pre-flight checks passed!")
	fmt.Println()
	return nil
}

// migrateInfraToIMDSv2 migrates infra nodes to IMDSv2 using the MachinePool dance.
// Returns true if changes were made, false if already configured.
func (o *imdsv2Options) migrateInfraToIMDSv2(ctx context.Context) (bool, error) {
	printer.PrintlnGreen("\n=== Migrating Infra Nodes to IMDSv2 ===")

	// Get the infra MachinePool from Hive
	infraMp, err := infra.GetInfraMachinePool(ctx, o.hiveClient, o.clusterID)
	if err != nil {
		return false, fmt.Errorf("failed to get infra MachinePool: %w", err)
	}

	// Validate MachinePool name (Comment #5: MachinePool matching safety)
	validMpNames := map[string]bool{"infra": true}
	if !validMpNames[infraMp.Spec.Name] {
		return false, fmt.Errorf("unexpected MachinePool name: %s (expected: infra)", infraMp.Spec.Name)
	}

	// Check if already configured for IMDSv2
	currentAuth := "Not configured"
	if infraMp.Spec.Platform.AWS != nil && infraMp.Spec.Platform.AWS.EC2Metadata != nil {
		currentAuth = infraMp.Spec.Platform.AWS.EC2Metadata.Authentication
	}

	if currentAuth == imdsv2Required {
		fmt.Println("Infra nodes already configured for IMDSv2 - skipping")
		return false, nil
	}

	// Display current state
	replicas := int64(2) // default
	if infraMp.Spec.Replicas != nil {
		replicas = *infraMp.Spec.Replicas
	}
	fmt.Printf("Current IMDS authentication: %s\n", currentAuth)
	fmt.Printf("Infra node count: %d\n", replicas)

	// Comment #3: Add confirmation prompt
	fmt.Printf("\nThis will replace all %d infra nodes using the MachinePool dance.\n", replicas)
	fmt.Println("During the process, there will temporarily be 2x infra nodes for high availability.")
	estimatedMinutes := int(replicas) * 10 // rough estimate
	fmt.Printf("Estimated time: ~%d minutes\n", estimatedMinutes)
	if !utils.ConfirmPrompt() {
		return false, errors.New("aborted by user")
	}

	// Clone the MachinePool and configure it for IMDSv2
	// NOTE: NO override annotation needed - the dance creates a new MP atomically
	newMp, err := infra.CloneMachinePool(infraMp, func(mp *hivev1.MachinePool) error {
		if mp.Spec.Platform.AWS == nil {
			mp.Spec.Platform.AWS = &awshivev1.MachinePoolPlatform{}
		}
		if mp.Spec.Platform.AWS.EC2Metadata == nil {
			mp.Spec.Platform.AWS.EC2Metadata = &awshivev1.EC2Metadata{}
		}
		mp.Spec.Platform.AWS.EC2Metadata.Authentication = imdsv2Required
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("failed to clone MachinePool: %w", err)
	}

	// Set up clients for the machinepool dance
	danceClients := infra.DanceClients{
		ClusterClient: o.client,
		HiveClient:    o.hiveClient,
		HiveAdmin:     o.hiveAdminClient,
	}

	// Comment #4 FIX: RunMachinePoolDance handles everything atomically
	// No annotations on the original MP, no cleanup needed
	fmt.Println("\nStarting MachinePool dance to replace infra nodes...")
	if err := infra.RunMachinePoolDance(ctx, danceClients, infraMp, newMp, nil); err != nil {
		return false, fmt.Errorf("MachinePool dance failed: %w", err)
	}

	// Wait for cluster operators to stabilize after replacement
	fmt.Println("\nWaiting for cluster operators to stabilize...")
	if err := WaitForClusterOperatorsHealthy(ctx, o.client, imdsv2COWaitTimeout); err != nil {
		return false, err
	}

	printer.PrintlnGreen("Infra nodes migrated to IMDSv2!")
	return true, nil
}

// migrateWorkersToIMDSv2 lists worker MachinePools that need IMDSv2 and asks user which to patch.
// Returns true if any changes were made, false if already configured or user skipped.
func (o *imdsv2Options) migrateWorkersToIMDSv2(ctx context.Context) (bool, error) {
	printer.PrintlnGreen("\n=== Worker Node MachinePools ===")

	// Get the Hive namespace for this cluster
	hiveNamespace, err := GetHiveNamespace(o.clusterID)
	if err != nil {
		return false, err
	}

	// Retrieve all MachinePools for this cluster
	mpList := &hivev1.MachinePoolList{}
	if err := o.hiveClient.List(ctx, mpList, &client.ListOptions{Namespace: hiveNamespace}); err != nil {
		return false, fmt.Errorf("failed to list MachinePools: %w", err)
	}

	// Find worker MachinePools that need IMDSv2
	type workerMPInfo struct {
		name         string
		replicas     int64
		instanceType string
		currentIMDS  string
	}
	var workersNeedingUpdate []workerMPInfo

	for _, mp := range mpList.Items {
		// Skip master and infra pools (exclusion-based approach for all worker pools)
		// This ensures we process all worker pools including custom ones like "worker-2", "gpu-workers", etc.
		if mp.Spec.Name == "master" || mp.Spec.Name == "infra" {
			continue
		}

		// Check current IMDSv2 configuration
		currentAuth := "Not configured"
		if mp.Spec.Platform.AWS != nil && mp.Spec.Platform.AWS.EC2Metadata != nil {
			currentAuth = mp.Spec.Platform.AWS.EC2Metadata.Authentication
		}

		if currentAuth != imdsv2Required {
			instanceType := "unknown"
			if mp.Spec.Platform.AWS != nil {
				instanceType = mp.Spec.Platform.AWS.InstanceType
			}

			replicas := int64(0)
			if mp.Spec.Replicas != nil {
				replicas = *mp.Spec.Replicas
			}

			workersNeedingUpdate = append(workersNeedingUpdate, workerMPInfo{
				name:         mp.Name,
				replicas:     replicas,
				instanceType: instanceType,
				currentIMDS:  currentAuth,
			})
		}
	}

	if len(workersNeedingUpdate) == 0 {
		fmt.Println("All worker MachinePools already configured for IMDSv2")
		return false, nil
	}

	// Display worker MachinePools that need IMDSv2
	fmt.Printf("\n%d worker MachinePool(s) requiring IMDSv2 configuration\n", len(workersNeedingUpdate))
	fmt.Println()

	// Ask for confirmation
	fmt.Println("NOTE: Worker node replacement must be performed by the customer.")
	fmt.Println("This will only PATCH the worker MachinePools to require IMDSv2.")
	fmt.Println("The customer must then delete worker machines to trigger replacement.")
	fmt.Printf("\nPatch %d worker MachinePool(s) to require IMDSv2?\n", len(workersNeedingUpdate))
	if !utils.ConfirmPrompt() {
		fmt.Println("Skipped - worker MachinePools not patched")
		return false, nil
	}

	// Patch each worker MachinePool
	anyPatched := false
	patchedCount := 0
	for _, mpInfo := range workersNeedingUpdate {
		patchedCount++
		fmt.Printf("\nPatching worker MachinePool %d of %d...\n", patchedCount, len(workersNeedingUpdate))

		// Get current MachinePool
		mp := &hivev1.MachinePool{}
		if err := o.hiveClient.Get(ctx, client.ObjectKey{Namespace: hiveNamespace, Name: mpInfo.name}, mp); err != nil {
			return false, fmt.Errorf("failed to get worker MachinePool %d of %d: %w", patchedCount, len(workersNeedingUpdate), err)
		}

		patch := client.MergeFrom(mp.DeepCopy())

		// Add override annotation to allow platform spec changes
		// NOTE: This is needed for in-place patching (unlike infra which uses MachinePool dance)
		if mp.Annotations == nil {
			mp.Annotations = make(map[string]string)
		}
		mp.Annotations[hiveOverrideAnnotation] = "true"

		// Configure IMDSv2 authentication requirement
		if mp.Spec.Platform.AWS == nil {
			mp.Spec.Platform.AWS = &awshivev1.MachinePoolPlatform{}
		}
		if mp.Spec.Platform.AWS.EC2Metadata == nil {
			mp.Spec.Platform.AWS.EC2Metadata = &awshivev1.EC2Metadata{}
		}
		mp.Spec.Platform.AWS.EC2Metadata.Authentication = imdsv2Required

		if err := o.hiveAdminClient.Patch(ctx, mp, patch); err != nil {
			return false, fmt.Errorf("failed to patch worker MachinePool %d of %d: %w", patchedCount, len(workersNeedingUpdate), err)
		}

		fmt.Printf("  ✓ MachinePool patched successfully\n")

		// Remove the override annotation now that the patch is applied
		// Re-fetch to get latest version before removing annotation
		if err := o.hiveClient.Get(ctx, client.ObjectKey{Namespace: hiveNamespace, Name: mpInfo.name}, mp); err != nil {
			return false, fmt.Errorf("failed to re-fetch worker MachinePool %d of %d: %w", patchedCount, len(workersNeedingUpdate), err)
		}

		patch = client.MergeFrom(mp.DeepCopy())
		delete(mp.Annotations, hiveOverrideAnnotation)

		if err := o.hiveAdminClient.Patch(ctx, mp, patch); err != nil {
			return false, fmt.Errorf("failed to remove override annotation from worker MachinePool %d of %d: %w", patchedCount, len(workersNeedingUpdate), err)
		}

		fmt.Printf("  ✓ Override annotation removed\n")
		anyPatched = true
	}

	if anyPatched {
		printer.PrintlnGreen("\nWorker MachinePools patched successfully!")
		fmt.Println("\n=== Next Steps (Customer Action Required) ===")
		fmt.Println("Worker nodes must be replaced by the customer using one of these methods:")
		fmt.Println("  1. Delete worker machines one at a time (MachineSet will replace with IMDSv2)")
		fmt.Println("  2. Scale down/up worker MachineSets")
		fmt.Println("  3. Use the MachinePool dance pattern (similar to infra node replacement)")
	}

	return anyPatched, nil
}

// updateCPMSForIMDSv2 patches the ControlPlaneMachineSet to trigger a rolling replacement.
// Returns true if changes were made, false if already configured.
func (o *imdsv2Options) updateCPMSForIMDSv2(ctx context.Context) (bool, error) {
	printer.PrintlnGreen("\n=== Updating ControlPlaneMachineSet for IMDSv2 ===")

	// Retrieve the ControlPlaneMachineSet
	cpms := &machinev1.ControlPlaneMachineSet{}
	if err := o.client.Get(ctx, client.ObjectKey{Namespace: cpmsNamespace, Name: cpmsName}, cpms); err != nil {
		return false, fmt.Errorf("failed to get CPMS: %w", err)
	}

	// Parse the AWS provider spec from CPMS template
	if cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value == nil {
		return false, fmt.Errorf("CPMS ProviderSpec.Value is nil")
	}

	awsSpec := &machinev1beta1.AWSMachineProviderConfig{}
	if err := json.Unmarshal(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value.Raw, awsSpec); err != nil {
		return false, fmt.Errorf("failed to unmarshal CPMS provider spec: %w", err)
	}

	// Skip if already configured for IMDSv2
	if awsSpec.MetadataServiceOptions.Authentication == imdsv2Required {
		fmt.Println("Control plane already configured for IMDSv2 - skipping")
		return false, nil
	}

	fmt.Printf("Current IMDS authentication: %s\n", awsSpec.MetadataServiceOptions.Authentication)
	fmt.Println("Patching CPMS to enforce IMDSv2...")

	// Confirm action with user (destructive operation)
	fmt.Printf("\nThis will replace all 3 control plane nodes one at a time (~35-45 min).\n")
	if !utils.ConfirmPrompt() {
		return false, errors.New("aborted by user")
	}

	// Update AWS spec to require IMDSv2
	awsSpec.MetadataServiceOptions.Authentication = imdsv2Required

	// Serialize and apply the updated spec
	rawBytes, err := json.Marshal(awsSpec)
	if err != nil {
		return false, fmt.Errorf("failed to marshal updated provider spec: %w", err)
	}

	patch := client.MergeFrom(cpms.DeepCopy())
	cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value = &runtime.RawExtension{Raw: rawBytes}

	// Apply the patch
	if err := o.clientAdmin.Patch(ctx, cpms, patch); err != nil {
		return false, fmt.Errorf("failed to patch CPMS: %w", err)
	}

	printer.PrintlnGreen("CPMS patched successfully. Rolling replacement in progress...")
	fmt.Println("Monitoring rollout (this may take 60-120 minutes)...")

	// Wait for all master nodes to be replaced
	if err := MonitorCPMSRollout(ctx, o.client, cpmsNamespace, cpmsName, imdsv2RolloutPollTimeout); err != nil {
		return false, err
	}

	printer.PrintlnGreen("Control plane IMDSv2 migration complete!")
	return true, nil
}

// validateIMDSv2 verifies the migration was successful.
func (o *imdsv2Options) validateIMDSv2(ctx context.Context) error {
	printer.PrintlnGreen("\n=== Validating IMDSv2 Migration ===")

	// Verify all nodes are in Ready state (except those being deleted)
	// Retry a few times to allow nodes to stabilize after replacement
	fmt.Println("Checking node status...")

	maxRetries := 5
	retryDelay := 30 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		nodes := &corev1.NodeList{}
		if err := o.client.List(ctx, nodes); err != nil {
			return fmt.Errorf("failed to list nodes: %w", err)
		}

		notReadyNodes := []string{}
		deletingNodes := []string{}
		unschedulableNodes := []string{}

		for _, node := range nodes.Items {
			// Skip nodes that are being deleted (have DeletionTimestamp set)
			if node.DeletionTimestamp != nil {
				deletingNodes = append(deletingNodes, node.Name)
				continue
			}

			// Skip nodes that are cordoned/unschedulable (being drained)
			if node.Spec.Unschedulable {
				unschedulableNodes = append(unschedulableNodes, node.Name)
				continue
			}

			ready := false
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
					ready = true
					break
				}
			}
			if !ready {
				notReadyNodes = append(notReadyNodes, node.Name)
			}
		}

		if len(deletingNodes) > 0 {
			fmt.Printf("  ⏳ %d node(s) being deleted\n", len(deletingNodes))
		}
		if len(unschedulableNodes) > 0 {
			fmt.Printf("  ⏳ %d node(s) being drained\n", len(unschedulableNodes))
		}

		// If all active nodes are ready, we're good
		if len(notReadyNodes) == 0 {
			activeNodes := len(nodes.Items) - len(deletingNodes) - len(unschedulableNodes)
			fmt.Printf("  ✓ All %d active nodes are Ready\n", activeNodes)
			break
		}

		// If we have NotReady nodes and haven't exhausted retries, wait and retry
		if attempt < maxRetries {
			fmt.Printf("  ⏳ Waiting for %d node(s) to become Ready (attempt %d/%d)\n",
				len(notReadyNodes), attempt, maxRetries)

			// Context-aware sleep to handle cancellation (SIGINT, timeout, etc.)
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled while waiting for nodes: %w", ctx.Err())
			case <-time.After(retryDelay):
				// Continue to next retry
			}
			continue
		}

		// Final attempt failed
		return fmt.Errorf("%d node(s) not Ready after %d attempts", len(notReadyNodes), maxRetries)
	}

	// Verify all machines have IMDSv2 configured in their spec
	fmt.Println("Checking machine configurations...")
	machines := &machinev1beta1.MachineList{}
	if err := o.client.List(ctx, machines, &client.ListOptions{Namespace: cpmsNamespace}); err != nil {
		return fmt.Errorf("failed to list machines: %w", err)
	}

	nonIMDSv2Machines := []string{}
	for _, machine := range machines.Items {
		if machine.Spec.ProviderSpec.Value == nil {
			continue
		}

		awsSpec := &machinev1beta1.AWSMachineProviderConfig{}
		if err := json.Unmarshal(machine.Spec.ProviderSpec.Value.Raw, awsSpec); err != nil {
			log.Printf("Warning: failed to unmarshal provider spec for a machine: %v", err)
			continue
		}

		if awsSpec.MetadataServiceOptions.Authentication != imdsv2Required {
			nonIMDSv2Machines = append(nonIMDSv2Machines, machine.Name)
		}
	}

	if len(nonIMDSv2Machines) > 0 {
		fmt.Printf("  ⚠ %d machine(s) not configured for IMDSv2\n", len(nonIMDSv2Machines))
		fmt.Println("  (This is expected for worker nodes - customer must replace them)")
	} else {
		fmt.Printf("  ✓ All %d machines configured for IMDSv2\n", len(machines.Items))
	}

	printer.PrintlnGreen("\nValidation complete!")
	return nil
}
