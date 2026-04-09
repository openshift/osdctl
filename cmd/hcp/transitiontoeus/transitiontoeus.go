package transitiontoeus

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/openshift-online/ocm-cli/pkg/arguments"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/internal/io"
	"github.com/openshift/osdctl/internal/servicelog"
	"github.com/openshift/osdctl/internal/utils"
	ocmutils "github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

// transitionOptions contains all options for the transition-to-eus command
type transitionOptions struct {
	clusterID    string
	clustersFile string
	dryRun       bool

	// Parsed cluster IDs (populated during validation)
	clusterIDs []string
}

// Service log template mappings
var serviceLogTemplates = map[string]string{
	"success":   "https://raw.githubusercontent.com/openshift/managed-notifications/master/hcp/eus_transition_success.json",
	"attempted": "https://raw.githubusercontent.com/openshift/managed-notifications/master/hcp/eus_transition_attempted.json",
}

// Template cache to avoid re-fetching the same templates in batch mode
var templateCache = make(map[string][]byte)

// Regular expression for valid cluster IDs - alphanumeric characters and hyphens only
var validClusterIDRegex = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

func newCmdTransitionToEUS() *cobra.Command {
	opts := &transitionOptions{}

	cmd := &cobra.Command{
		Use:   "transition-to-eus",
		Short: "Transition ROSA HCP clusters from stable to EUS channel (Even Y-Stream EOL handling)",
		Long: `Transition ROSA HCP clusters from stable to EUS channel during End-of-Life handling.

⚠️ IMPORTANT GUARDRAILS ⚠️
This command is specifically designed for EVEN Y-STREAM end-of-life transitions (4.14, 4.16, 4.18, etc.).

The command validates:
- Cluster must be HCP (not Classic)
- Cluster must be on an even y-stream (4.14, 4.16, 4.18, etc.)
- Cluster must be on 'stable' channel (not already on 'eus')
- Cluster must be in 'ready' state

WORKFLOW:
For clusters with recurring update policies:
1. Saves the existing recurring update policy settings
2. Deletes the recurring update policy
3. Transitions the channel from 'stable' to 'eus'
4. Verifies the channel change
5. Restores the recurring update policy with original settings
6. Prompts to send service log notification

For clusters with individual updates:
1. Transitions the channel from 'stable' to 'eus'
2. Verifies the channel change
3. Prompts to send service log notification

SERVICE LOG BEHAVIOR:
- After each successful transition, you will be prompted to optionally send a service log notification
- On failures where recurring policy was modified and restored: You will be prompted to send an 'attempted' notification to the customer

This approach extends the support lifecycle for clusters on even y-streams without forcing upgrades.`,
		Example: `  # Transition single cluster (will prompt to send service log after success)
  osdctl hcp transition-to-eus -C cluster123

  # Multiple clusters from file
  osdctl hcp transition-to-eus --clusters-file clusters.json

  # Dry-run to preview changes
  osdctl hcp transition-to-eus --clusters-file clusters.json --dry-run
`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.Run()
		},
	}

	// Cluster targeting flags
	cmd.Flags().StringVarP(&opts.clusterID, "cluster-id", "C", "", "ID of the target HCP cluster")
	cmd.Flags().StringVarP(&opts.clustersFile, "clusters-file", "c", "", "JSON file containing cluster IDs (format: {\"clusters\":[\"$CLUSTERID1\", \"$CLUSTERID2\"]})")

	// Configuration flags
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Simulate the transition without making any changes")

	return cmd
}

func (o *transitionOptions) validate() error {
	// Exactly one cluster targeting method must be provided
	if o.clusterID == "" && o.clustersFile == "" {
		return fmt.Errorf("no cluster identifier has been found, please specify either --cluster-id or --clusters-file")
	}

	if o.clusterID != "" && o.clustersFile != "" {
		return fmt.Errorf("cannot specify both --cluster-id and --clusters-file, choose one")
	}

	// Validate cluster ID format when using single cluster ID
	if o.clusterID != "" {
		if !validClusterIDRegex.MatchString(o.clusterID) {
			return fmt.Errorf("cluster ID '%s' contains invalid characters - only alphanumeric characters and hyphens are allowed", o.clusterID)
		}
	}

	// Parse and validate cluster targets
	if o.clustersFile != "" {
		clusterIDs, err := io.ParseAndValidateClustersFile(o.clustersFile)
		if err != nil {
			return err
		}
		// Require at least one cluster
		if len(clusterIDs) == 0 {
			return fmt.Errorf("clusters file contains no cluster IDs - the 'clusters' array is empty")
		}
		o.clusterIDs = clusterIDs
	} else {
		o.clusterIDs = []string{o.clusterID}
	}

	return nil
}

func (o *transitionOptions) Run() error {
	if err := o.validate(); err != nil {
		return err
	}

	ocmClient, err := ocmutils.CreateConnection()
	if err != nil {
		return fmt.Errorf("failed to create OCM connection: %w", err)
	}
	defer ocmClient.Close()

	clusters, err := o.getClusters(ocmClient)
	if err != nil {
		return fmt.Errorf("failed to get target clusters: %w", err)
	}

	if len(clusters) == 0 {
		fmt.Println("No clusters found matching the given cluster-id or cluster list.")
		return nil
	}

	// Pre-validate clusters and filter out those that don't need transition
	eligibleClusters, skippedClusters := o.preValidateClusters(clusters)

	// Show skipped clusters (already on EUS, validation failures, etc.)
	if len(skippedClusters) > 0 {
		fmt.Printf("\nℹ️  Skipping %d cluster(s) that don't require transition:\n", len(skippedClusters))
		for _, skip := range skippedClusters {
			fmt.Printf("  • %s (%s): %s\n", skip.cluster.ExternalID(), skip.cluster.Name(), skip.reason)
		}
		fmt.Println()
	}

	// If no eligible clusters, exit early
	if len(eligibleClusters) == 0 {
		fmt.Println("✓ No clusters require transition. All clusters are already on EUS or don't meet criteria.")
		return nil
	}

	// Display cluster list and service log preview before processing
	if err := o.printPreProcessingSummary(eligibleClusters); err != nil {
		return fmt.Errorf("failed to display pre-processing summary: %w", err)
	}

	// Ask for confirmation before proceeding (unless in dry-run mode)
	if !o.dryRun {
		if !ocmutils.ConfirmPrompt() {
			fmt.Println("EUS transition operation cancelled.")
			return nil
		}
		fmt.Println()
	}

	// Process only eligible clusters
	clusters = eligibleClusters

	var successful, failed []string

	for i, cluster := range clusters {
		fmt.Printf("\n[%d/%d] Processing cluster: %s (%s)\n", i+1, len(clusters), cluster.ID(), cluster.Name())

		result := o.processCluster(ocmClient, cluster)

		if result.err != nil {
			failed = append(failed, fmt.Sprintf("%s: %s", cluster.ExternalID(), result.err.Error()))
			fmt.Printf("  ⚠️  Failed to transition: %v\n", result.err)

			// Prompt to send "attempted" service log if we modified recurring policy and restored it
			// This means we actually changed the customer's cluster configuration but transition failed
			if result.policyWasModified && result.policyWasRestored {
				if !o.dryRun {
					fmt.Println()
					fmt.Printf("  ℹ️  Note: The recurring upgrade policy was modified during this attempt but has been restored.\n")

					// Show service log preview and prompt
					if err := promptAndSendServiceLog(ocmClient, cluster, "attempted"); err != nil {
						fmt.Printf("  ⚠️  Failed to send service log: %v\n", err)
					}
				} else {
					fmt.Printf("  📧 DRY-RUN: Would prompt to send 'attempted' service log (policy was modified and restored)\n")
				}
			}
		} else {
			successful = append(successful, cluster.ExternalID())

			// Prompt user to send service log after successful transition
			if !o.dryRun {
				fmt.Println()
				fmt.Printf("  ✅ Transition completed successfully!\n")

				// Show service log preview and prompt
				if err := promptAndSendServiceLog(ocmClient, cluster, "success"); err != nil {
					fmt.Printf("  ⚠️  Failed to send service log: %v\n", err)
				}
			} else {
				fmt.Printf("  📧 DRY-RUN: Would prompt to send service log notification\n")
			}
		}
	}

	o.printSummary(successful, failed)
	return nil
}

func (o *transitionOptions) getClusters(ocmClient *sdk.Connection) ([]*v1.Cluster, error) {
	clusterIDs := o.clusterIDs

	var queries []string
	for _, id := range clusterIDs {
		if id == "" {
			return nil, fmt.Errorf("encountered empty cluster ID, this should not happen")
		}
		queries = append(queries, ocmutils.GenerateQuery(id))
	}

	// Ensure we have at least one valid query
	if len(queries) == 0 {
		return nil, fmt.Errorf("no valid cluster IDs found to create OCM search query")
	}

	clusters, err := ocmutils.ApplyFilters(ocmClient, []string{strings.Join(queries, " or ")})
	if err != nil {
		return nil, fmt.Errorf("failed to find clusters: %w", err)
	}

	if len(clusterIDs) != len(clusters) {
		fmt.Println("")
		fmt.Printf("⚠️ Warning: found %d clusters but expected %d. This can happen when clusters are no longer available in OCM, e.g. due to a deletion.\n", len(clusterIDs), len(clusters))
		fmt.Println("")
	}

	return clusters, nil
}

// skippedCluster represents a cluster that was skipped with a reason
type skippedCluster struct {
	cluster *v1.Cluster
	reason  string
}

// preValidateClusters validates all clusters upfront and returns eligible vs skipped clusters
func (o *transitionOptions) preValidateClusters(clusters []*v1.Cluster) ([]*v1.Cluster, []skippedCluster) {
	var eligible []*v1.Cluster
	var skipped []skippedCluster

	for _, cluster := range clusters {
		// Validation 1: Must be HCP cluster
		if !cluster.Hypershift().Enabled() {
			skipped = append(skipped, skippedCluster{
				cluster: cluster,
				reason:  "not an HCP cluster (Classic clusters are not supported)",
			})
			continue
		}

		// Validation 2: Must be on even y-stream
		version, err := semver.NewVersion(cluster.OpenshiftVersion())
		if err != nil {
			skipped = append(skipped, skippedCluster{
				cluster: cluster,
				reason:  fmt.Sprintf("invalid version format '%s'", cluster.OpenshiftVersion()),
			})
			continue
		}

		if version.Minor()%2 != 0 {
			skipped = append(skipped, skippedCluster{
				cluster: cluster,
				reason:  fmt.Sprintf("odd y-stream %d.%d (only even y-streams 4.14, 4.16, 4.18, etc. are supported)", version.Major(), version.Minor()),
			})
			continue
		}

		// Validation 3: Must be in ready state
		if cluster.State() != v1.ClusterStateReady {
			skipped = append(skipped, skippedCluster{
				cluster: cluster,
				reason:  fmt.Sprintf("cluster state is '%s' (must be 'ready')", cluster.State()),
			})
			continue
		}

		// Validation 4: Must be on stable channel (not already on eus)
		currentChannel := cluster.Version().ChannelGroup()
		if currentChannel == "eus" {
			skipped = append(skipped, skippedCluster{
				cluster: cluster,
				reason:  "already on 'eus' channel (no transition needed)",
			})
			continue
		}

		if currentChannel != "stable" {
			skipped = append(skipped, skippedCluster{
				cluster: cluster,
				reason:  fmt.Sprintf("on '%s' channel (expected 'stable' channel for EUS transition)", currentChannel),
			})
			continue
		}

		// All validations passed - cluster is eligible
		eligible = append(eligible, cluster)
	}

	return eligible, skipped
}

// recurringPolicyBackup stores complete information needed to restore a recurring upgrade policy
type recurringPolicyBackup struct {
	id           string
	schedule     string
	scheduleType v1.ScheduleType
	upgradeType  v1.UpgradeType
	enableMinor  bool
	version      string // May be empty for automatic policies
}

// policyDetails stores information about a cluster's upgrade policies
type policyDetails struct {
	hasRecurringPolicies bool
	recurringPolicies    []recurringPolicyBackup
}

// clusterProcessResult contains the result of processing a cluster
type clusterProcessResult struct {
	err               error
	policyWasModified bool // true if we deleted recurring policy (even if later restored)
	policyWasRestored bool // true if we successfully restored the policy after modification
}

func (o *transitionOptions) processCluster(ocmClient *sdk.Connection, cluster *v1.Cluster) *clusterProcessResult {
	result := &clusterProcessResult{}

	// Get cluster version info (already validated in preValidateClusters)
	version, err := semver.NewVersion(cluster.OpenshiftVersion())
	if err != nil {
		result.err = fmt.Errorf("failed to parse cluster version '%s': %w", cluster.OpenshiftVersion(), err)
		return result
	}

	fmt.Printf("  ✓ Cluster is HCP on even y-stream %d.%d\n", version.Major(), version.Minor())

	currentChannel := cluster.Version().ChannelGroup()
	fmt.Printf("  ✓ Cluster is on 'stable' channel and ready to transition\n")

	// Step 1: Check for recurring upgrade policies
	policy, err := o.getUpgradePolicyDetails(ocmClient, cluster)
	if err != nil {
		result.err = fmt.Errorf("failed to check upgrade policies: %w", err)
		return result
	}

	// Step 2: Delete recurring policies if they exist
	if policy.hasRecurringPolicies {
		if len(policy.recurringPolicies) == 1 {
			fmt.Printf("  📋 Found recurring update policy (schedule: %s)\n", policy.recurringPolicies[0].schedule)
		} else {
			fmt.Printf("  📋 Found %d recurring update policies\n", len(policy.recurringPolicies))
		}

		if o.dryRun {
			fmt.Printf("  🔍 DRY-RUN: Would delete %d upgrade policy/policies\n", len(policy.recurringPolicies))
		} else {
			fmt.Printf("  🗑️  Deleting %d upgrade policy/policies to allow channel transition\n", len(policy.recurringPolicies))
			deleted, err := o.deleteUpgradePolicies(ocmClient, cluster, policy.recurringPolicies)
			// Mark if we deleted any policies (even if later ones failed)
			if len(deleted) > 0 {
				result.policyWasModified = true
			}
			if err != nil {
				// If we partially deleted policies, try to restore what we deleted
				if len(deleted) > 0 {
					fmt.Printf("  🔁 Deletion failed after deleting %d/%d policies - attempting to restore deleted policies\n", len(deleted), len(policy.recurringPolicies))
					if restoreErr := o.restoreUpgradePolicies(ocmClient, cluster, deleted); restoreErr != nil {
						result.err = fmt.Errorf("failed to delete upgrade policies: %w (CRITICAL: also failed to restore %d deleted policies: %v)", err, len(deleted), restoreErr)
					} else {
						result.policyWasRestored = true
						result.err = fmt.Errorf("failed to delete upgrade policies: %w (deleted policies restored)", err)
						fmt.Printf("  ✓ Deleted policies restored after failure\n")
					}
				} else {
					result.err = fmt.Errorf("failed to delete upgrade policies: %w", err)
				}
				return result
			}
		}
	} else {
		fmt.Printf("  ℹ️  Cluster uses individual updates (no recurring policies to delete)\n")
	}

	// Step 3: Transition channel to EUS
	if o.dryRun {
		fmt.Printf("  🔍 DRY-RUN: Would transition channel from '%s' to 'eus'\n", currentChannel)
	} else {
		fmt.Printf("  🔄 Transitioning channel from '%s' to 'eus'\n", currentChannel)
		if err := o.transitionChannelToEUS(ocmClient, cluster); err != nil {
			// Channel transition failed - try to restore policies if we deleted them
			if result.policyWasModified {
				fmt.Printf("  🔁 Channel transition failed - attempting to restore upgrade policies\n")
				if restoreErr := o.restoreUpgradePolicies(ocmClient, cluster, policy.recurringPolicies); restoreErr != nil {
					result.err = fmt.Errorf("failed to transition channel: %w (CRITICAL: also failed to restore policies: %v)", err, restoreErr)
				} else {
					result.policyWasRestored = true
					result.err = fmt.Errorf("failed to transition channel: %w (upgrade policies restored)", err)
					fmt.Printf("  ✓ Upgrade policies restored after failure\n")
				}
			} else {
				result.err = fmt.Errorf("failed to transition channel: %w", err)
			}
			return result
		}

		// Step 4: Verify channel change with bounded polling
		fmt.Printf("  🔍 Verifying channel transition\n")
		newChannel, err := o.pollChannelChange(ocmClient, cluster, "eus", 10, 2*time.Second)
		if err != nil {
			// Verification failed - try to restore policies if we deleted them
			if result.policyWasModified {
				fmt.Printf("  🔁 Channel verification failed - attempting to restore upgrade policies\n")
				if restoreErr := o.restoreUpgradePolicies(ocmClient, cluster, policy.recurringPolicies); restoreErr != nil {
					result.err = fmt.Errorf("failed to verify channel change: %w (CRITICAL: also failed to restore policies: %v)", err, restoreErr)
				} else {
					result.policyWasRestored = true
					result.err = fmt.Errorf("failed to verify channel change: %w (upgrade policies restored)", err)
					fmt.Printf("  ✓ Upgrade policies restored after failure\n")
				}
			} else {
				result.err = fmt.Errorf("failed to verify channel change: %w", err)
			}
			return result
		}

		fmt.Printf("  ✓ Channel successfully transitioned to 'eus' (verified: %s)\n", newChannel)
	}

	// Step 5: Restore recurring policies if they existed
	if policy.hasRecurringPolicies {
		if o.dryRun {
			fmt.Printf("  🔍 DRY-RUN: Would restore %d upgrade policy/policies\n", len(policy.recurringPolicies))
		} else {
			fmt.Printf("  🔁 Restoring %d upgrade policy/policies\n", len(policy.recurringPolicies))
			if err := o.restoreUpgradePolicies(ocmClient, cluster, policy.recurringPolicies); err != nil {
				result.err = fmt.Errorf("failed to restore upgrade policies: %w", err)
				return result
			}
			result.policyWasRestored = true
			fmt.Printf("  ✓ Upgrade policies restored\n")
		}
	}

	if !o.dryRun {
		fmt.Printf("  ✅ Successfully transitioned cluster to EUS channel\n")
	} else {
		fmt.Printf("  🔍 DRY-RUN: Transition would be successful\n")
	}

	return result
}

func (o *transitionOptions) getUpgradePolicyDetails(ocmClient *sdk.Connection, cluster *v1.Cluster) (*policyDetails, error) {
	policiesResponse, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(cluster.ID()).
		ControlPlane().UpgradePolicies().List().Send()
	if err != nil {
		return nil, fmt.Errorf("failed to list upgrade policies: %w", err)
	}

	policies := policiesResponse.Items().Slice()
	if len(policies) == 0 {
		return &policyDetails{hasRecurringPolicies: false}, nil
	}

	// Filter and preserve all recurring (automatic) policies - ignore one-off manual upgrade policies
	// One-off manual policies (ScheduleTypeManual) are customer-scheduled upgrades that should not be touched
	var recurringPolicies []recurringPolicyBackup
	for _, policy := range policies {
		if policy.ScheduleType() == v1.ScheduleTypeAutomatic {
			// Preserve complete policy details for faithful restoration
			backup := recurringPolicyBackup{
				id:           policy.ID(),
				schedule:     policy.Schedule(),
				scheduleType: policy.ScheduleType(),
				upgradeType:  policy.UpgradeType(),
				enableMinor:  policy.EnableMinorVersionUpgrades(),
			}
			// Version may be empty for automatic policies
			if policy.Version() != "" {
				backup.version = policy.Version()
			}
			recurringPolicies = append(recurringPolicies, backup)
		}
		// Skip manual one-off policies - these are customer-scheduled and should not be deleted
	}

	// No recurring policies found - cluster uses individual updates or only has one-off manual upgrades
	if len(recurringPolicies) == 0 {
		return &policyDetails{hasRecurringPolicies: false}, nil
	}

	return &policyDetails{
		hasRecurringPolicies: true,
		recurringPolicies:    recurringPolicies,
	}, nil
}

func (o *transitionOptions) deleteUpgradePolicies(ocmClient *sdk.Connection, cluster *v1.Cluster, policies []recurringPolicyBackup) ([]recurringPolicyBackup, error) {
	deleted := make([]recurringPolicyBackup, 0, len(policies))
	for _, policy := range policies {
		_, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(cluster.ID()).
			ControlPlane().UpgradePolicies().ControlPlaneUpgradePolicy(policy.id).Delete().Send()
		if err != nil {
			return deleted, fmt.Errorf("failed to delete policy %s: %w", policy.id, err)
		}
		deleted = append(deleted, policy)
	}
	return deleted, nil
}

func (o *transitionOptions) transitionChannelToEUS(ocmClient *sdk.Connection, cluster *v1.Cluster) error {
	versionBuilder := v1.NewVersion().ChannelGroup("eus")

	clusterUpdate, err := v1.NewCluster().Version(versionBuilder).Build()
	if err != nil {
		return fmt.Errorf("failed to build cluster update: %w", err)
	}

	_, err = ocmClient.ClustersMgmt().V1().Clusters().Cluster(cluster.ID()).Update().Body(clusterUpdate).Send()
	return err
}

func (o *transitionOptions) restoreUpgradePolicies(ocmClient *sdk.Connection, cluster *v1.Cluster, policies []recurringPolicyBackup) error {
	// Track newly created policy IDs for rollback if needed
	var restoredPolicyIDs []string

	for i, backup := range policies {
		// Build policy with all preserved fields
		policyBuilder := v1.NewControlPlaneUpgradePolicy().
			Schedule(backup.schedule).
			ScheduleType(backup.scheduleType).
			UpgradeType(backup.upgradeType).
			EnableMinorVersionUpgrades(backup.enableMinor)

		// Add version only for manual policies - automatic policies must not have version set
		// Manual policies target a specific version, automatic policies use the latest available
		if backup.version != "" && backup.scheduleType == v1.ScheduleTypeManual {
			policyBuilder = policyBuilder.Version(backup.version)
		}

		newPolicy, err := policyBuilder.Build()
		if err != nil {
			// Rollback: delete any policies we've already restored
			if len(restoredPolicyIDs) > 0 {
				o.rollbackRestoredPolicies(ocmClient, cluster, restoredPolicyIDs)
			}
			return fmt.Errorf("failed to build upgrade policy %d/%d: %w", i+1, len(policies), err)
		}

		response, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(cluster.ID()).
			ControlPlane().UpgradePolicies().Add().Body(newPolicy).Send()
		if err != nil {
			// Rollback: delete any policies we've already restored
			if len(restoredPolicyIDs) > 0 {
				o.rollbackRestoredPolicies(ocmClient, cluster, restoredPolicyIDs)
			}
			return fmt.Errorf("failed to restore policy %d/%d: %w", i+1, len(policies), err)
		}

		// Track the newly created policy ID for potential rollback
		restoredPolicyIDs = append(restoredPolicyIDs, response.Body().ID())
	}
	return nil
}

// rollbackRestoredPolicies deletes policies that were just created during a failed restore operation
func (o *transitionOptions) rollbackRestoredPolicies(ocmClient *sdk.Connection, cluster *v1.Cluster, policyIDs []string) {
	fmt.Printf("  🔁 Rolling back %d partially restored policies\n", len(policyIDs))
	for _, policyID := range policyIDs {
		_, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(cluster.ID()).
			ControlPlane().UpgradePolicies().ControlPlaneUpgradePolicy(policyID).Delete().Send()
		if err != nil {
			fmt.Printf("  ⚠️  Warning: failed to rollback policy %s during restore failure: %v\n", policyID, err)
		}
	}
}

// pollChannelChange polls OCM to verify the channel change with bounded retries
func (o *transitionOptions) pollChannelChange(ocmClient *sdk.Connection, cluster *v1.Cluster, expectedChannel string, maxAttempts int, interval time.Duration) (string, error) {
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		time.Sleep(interval)

		updatedCluster, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(cluster.ID()).Get().Send()
		if err != nil {
			// Only fail on last attempt, otherwise retry
			if attempt == maxAttempts {
				return "", fmt.Errorf("failed to get cluster after %d attempts: %w", maxAttempts, err)
			}
			continue
		}

		currentChannel := updatedCluster.Body().Version().ChannelGroup()
		if currentChannel == expectedChannel {
			return currentChannel, nil
		}

		// Only fail on last attempt
		if attempt == maxAttempts {
			return currentChannel, fmt.Errorf("channel verification failed after %d attempts - expected '%s' but got '%s'", maxAttempts, expectedChannel, currentChannel)
		}
	}

	return "", fmt.Errorf("unexpected error in polling loop")
}

// loadServiceLogTemplate loads a service log template from either a predefined template name or file path
// Templates are cached to avoid re-fetching in batch mode
func loadServiceLogTemplate(templateOrFile string) ([]byte, bool, error) {
	var templateBytes []byte
	var err error
	var usingDefaultTemplate bool

	// Check if it's a template name
	if templateURL, exists := serviceLogTemplates[templateOrFile]; exists {
		// Check cache first
		if cached, found := templateCache[templateOrFile]; found {
			return cached, true, nil
		}

		// Fetch and cache the template
		templateBytes, err = utils.CurlThis(templateURL)
		if err != nil {
			return nil, false, fmt.Errorf("failed to fetch template from %s: %w", templateURL, err)
		}
		templateCache[templateOrFile] = templateBytes
		usingDefaultTemplate = true
	} else {
		// Treat as file path - don't cache file-based templates as they may be edited between runs
		templateBytes, err = os.ReadFile(templateOrFile)
		if err != nil {
			return nil, false, fmt.Errorf("failed to read template file %s: %w", templateOrFile, err)
		}
		usingDefaultTemplate = false
	}

	return templateBytes, usingDefaultTemplate, nil
}

// promptAndSendServiceLog shows the service log preview and prompts user to send it
func promptAndSendServiceLog(ocmClient *sdk.Connection, cluster *v1.Cluster, templateName string) error {
	// Load and prepare the message
	templateBytes, usingDefaultTemplate, err := loadServiceLogTemplate(templateName)
	if err != nil {
		return err
	}

	var message servicelog.Message
	if err := json.Unmarshal(templateBytes, &message); err != nil {
		return fmt.Errorf("failed to parse service log template: %w", err)
	}

	// Set cluster-specific fields (matching cmd/servicelog/post.go:612-617)
	message.ClusterUUID = cluster.ExternalID()
	message.ClusterID = cluster.ID()
	if subscription := cluster.Subscription(); subscription != nil {
		message.SubscriptionID = subscription.ID()
	}

	// Validate that all required parameters were replaced
	if leftoverParams, found := message.FindLeftovers(); found {
		if usingDefaultTemplate {
			return fmt.Errorf("default template contains unresolved parameters: %v. This should not happen", leftoverParams)
		} else {
			return fmt.Errorf("custom template contains unresolved parameters: %v. Please ensure all parameters are defined in your template", leftoverParams)
		}
	}

	// Display service log preview
	fmt.Println()
	fmt.Printf("  📧 SERVICE LOG PREVIEW:\n")
	fmt.Printf("  Summary:  %s\n", message.Summary)
	fmt.Printf("  Severity: %s\n", message.Severity)
	fmt.Println()
	fmt.Printf("  Message to Customer:\n")
	fmt.Printf("  %s\n", message.Description)
	fmt.Println()

	// Prompt user
	if templateName == "success" {
		fmt.Printf("  📧 Do you want to send this service log notification to the customer? (yes/no): ")
	} else {
		fmt.Printf("  📧 Do you want to send this 'attempted' service log to notify the customer? (yes/no): ")
	}

	if ocmutils.ConfirmPrompt() {
		fmt.Printf("  📄 Sending '%s' service log template\n", templateName)
		if err := sendServiceLog(ocmClient, &message); err != nil {
			return err
		}
		fmt.Printf("  📧 Service log notification sent successfully\n")
	} else {
		fmt.Printf("  ℹ️  Skipping service log notification\n")
	}

	return nil
}

// sendServiceLog sends the prepared service log message
// Implementation aligned with cmd/servicelog/post.go:304-310 and common.go:24-56
func sendServiceLog(ocmClient *sdk.Connection, message *servicelog.Message) error {
	request := ocmClient.Post()
	if err := arguments.ApplyPathArg(request, "/api/service_logs/v1/cluster_logs"); err != nil {
		return fmt.Errorf("cannot parse API path: %v", err)
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("cannot marshal service log message: %v", err)
	}

	request.Bytes(messageBytes)

	response, err := ocmutils.SendRequest(request)
	if err != nil {
		return fmt.Errorf("failed to send service log: %w", err)
	}

	body := response.Bytes()

	// Validate response body - match cmd/servicelog/post.go:339-356
	if response.Status() < 400 {
		// Success response - validate that API echoed back expected fields
		if err := validateServiceLogResponse(body, *message); err != nil {
			return fmt.Errorf("service log sent but response validation failed: %w", err)
		}
		return nil
	}

	// Error response - parse and return the error reason
	badReply, err := validateBadServiceLogResponse(body)
	if err != nil {
		return fmt.Errorf("service log request failed with status %d: %w", response.Status(), err)
	}
	return fmt.Errorf("service log request failed: %s (status: %d)", badReply.Reason, response.Status())
}

// validateServiceLogResponse validates that the API response echoes back the expected message fields
// Implementation from cmd/servicelog/common.go:24-50
func validateServiceLogResponse(body []byte, clusterMessage servicelog.Message) error {
	if !json.Valid(body) {
		return fmt.Errorf("server returned invalid JSON")
	}

	var goodReply servicelog.GoodReply
	if err := json.Unmarshal(body, &goodReply); err != nil {
		return fmt.Errorf("cannot parse the JSON response: %w", err)
	}

	// Validate that critical fields match what we sent
	if goodReply.Severity != clusterMessage.Severity {
		return fmt.Errorf("wrong severity echoed (sent %q, got %q)", clusterMessage.Severity, goodReply.Severity)
	}
	if goodReply.ServiceName != clusterMessage.ServiceName {
		return fmt.Errorf("wrong service_name echoed (sent %q, got %q)", clusterMessage.ServiceName, goodReply.ServiceName)
	}
	if goodReply.ClusterUUID != clusterMessage.ClusterUUID {
		return fmt.Errorf("wrong cluster_uuid echoed (sent %q, got %q)", clusterMessage.ClusterUUID, goodReply.ClusterUUID)
	}
	if goodReply.Summary != clusterMessage.Summary {
		return fmt.Errorf("wrong summary echoed (sent %q, got %q)", clusterMessage.Summary, goodReply.Summary)
	}
	if goodReply.Description != clusterMessage.Description {
		return fmt.Errorf("wrong description echoed (sent %q, got %q)", clusterMessage.Description, goodReply.Description)
	}

	return nil
}

// validateBadServiceLogResponse parses error response body
// Implementation from cmd/servicelog/common.go:52-59
func validateBadServiceLogResponse(body []byte) (*servicelog.BadReply, error) {
	if !json.Valid(body) {
		return nil, fmt.Errorf("server returned invalid JSON")
	}

	var badReply servicelog.BadReply
	if err := json.Unmarshal(body, &badReply); err != nil {
		return nil, fmt.Errorf("cannot parse error JSON response: %w", err)
	}

	return &badReply, nil
}

func (o *transitionOptions) printSummary(successful, failed []string) {
	fmt.Print("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Print("EUS TRANSITION SUMMARY\n")
	fmt.Print(strings.Repeat("=", 60) + "\n")

	total := len(successful) + len(failed)
	fmt.Printf("Total clusters processed: %d\n", total)
	fmt.Printf("Successfully transitioned: %d\n", len(successful))
	fmt.Printf("Failed: %d\n", len(failed))

	if len(failed) > 0 {
		fmt.Printf("\n⚠️ Failed to transition the following clusters (please follow-up manually):\n")
		for _, entry := range failed {
			fmt.Printf("  - %s\n", entry)
		}
	}

	fmt.Print(strings.Repeat("=", 60) + "\n")
}

// printPreProcessingSummary displays the clusters before processing
func (o *transitionOptions) printPreProcessingSummary(clusters []*v1.Cluster) error {
	fmt.Print("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Print("PRE-PROCESSING SUMMARY\n")
	fmt.Print(strings.Repeat("=", 60) + "\n")

	fmt.Printf("\nClusters to transition from 'stable' to 'eus' channel:\n")
	for i, cluster := range clusters {
		fmt.Printf("%2d. %s (%s) - %s - %s - Channel: %s\n",
			i+1,
			cluster.ExternalID(),
			cluster.Name(),
			cluster.State(),
			cluster.OpenshiftVersion(),
			cluster.Version().ChannelGroup(),
		)
	}

	if !o.dryRun {
		fmt.Printf("\nService Log Behavior:\n")
		fmt.Printf("  You will be prompted after each successful transition to send a service log notification.\n")
	}

	fmt.Print(strings.Repeat("=", 60) + "\n")

	return nil
}
