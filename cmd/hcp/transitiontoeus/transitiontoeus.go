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
	"success":   "https://raw.githubusercontent.com/diakovnec/managed-notifications/refs/heads/ohss-49572-notification/hcp/eus_transition_success.json",
	"attempted": "https://raw.githubusercontent.com/diakovnec/managed-notifications/refs/heads/ohss-49572-notification/hcp/eus_transition_attempted.json",
}

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

// policyDetails stores information about a cluster's upgrade policy
type policyDetails struct {
	hasRecurringPolicy bool
	policyID           string
	schedule           string
	scheduleType       string
	enableMinor        bool
}

// clusterProcessResult contains the result of processing a cluster
type clusterProcessResult struct {
	err                  error
	policyWasModified    bool // true if we deleted recurring policy (even if later restored)
	policyWasRestored    bool // true if we successfully restored the policy after modification
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

	// Step 2: Delete recurring policy if it exists
	if policy.hasRecurringPolicy {
		fmt.Printf("  📋 Found recurring update policy (schedule: %s)\n", policy.schedule)

		if o.dryRun {
			fmt.Printf("  🔍 DRY-RUN: Would delete upgrade policy\n")
		} else {
			fmt.Printf("  🗑️  Deleting upgrade policy to allow channel transition\n")
			if err := o.deleteUpgradePolicy(ocmClient, cluster, policy.policyID); err != nil {
				result.err = fmt.Errorf("failed to delete upgrade policy: %w", err)
				return result
			}
			// Mark that we've modified the policy
			result.policyWasModified = true
		}
	} else {
		fmt.Printf("  ℹ️  Cluster uses individual updates (no policy to delete)\n")
	}

	// Step 3: Transition channel to EUS
	if o.dryRun {
		fmt.Printf("  🔍 DRY-RUN: Would transition channel from '%s' to 'eus'\n", currentChannel)
	} else {
		fmt.Printf("  🔄 Transitioning channel from '%s' to 'eus'\n", currentChannel)
		if err := o.transitionChannelToEUS(ocmClient, cluster); err != nil {
			// Channel transition failed - try to restore policy if we deleted it
			if result.policyWasModified {
				fmt.Printf("  🔁 Channel transition failed - attempting to restore upgrade policy\n")
				if restoreErr := o.restoreUpgradePolicy(ocmClient, cluster, policy); restoreErr != nil {
					result.err = fmt.Errorf("failed to transition channel: %w (CRITICAL: also failed to restore policy: %v)", err, restoreErr)
				} else {
					result.policyWasRestored = true
					result.err = fmt.Errorf("failed to transition channel: %w (upgrade policy was restored)", err)
					fmt.Printf("  ✓ Upgrade policy restored after failure\n")
				}
			} else {
				result.err = fmt.Errorf("failed to transition channel: %w", err)
			}
			return result
		}

		// Step 4: Verify channel change
		time.Sleep(2 * time.Second) // Give OCM time to process
		updatedCluster, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(cluster.ID()).Get().Send()
		if err != nil {
			// Verification failed - try to restore policy if we deleted it
			if result.policyWasModified {
				fmt.Printf("  🔁 Channel verification failed - attempting to restore upgrade policy\n")
				if restoreErr := o.restoreUpgradePolicy(ocmClient, cluster, policy); restoreErr != nil {
					result.err = fmt.Errorf("failed to verify channel change: %w (CRITICAL: also failed to restore policy: %v)", err, restoreErr)
				} else {
					result.policyWasRestored = true
					result.err = fmt.Errorf("failed to verify channel change: %w (upgrade policy was restored)", err)
					fmt.Printf("  ✓ Upgrade policy restored after failure\n")
				}
			} else {
				result.err = fmt.Errorf("failed to verify channel change: %w", err)
			}
			return result
		}

		newChannel := updatedCluster.Body().Version().ChannelGroup()
		if newChannel != "eus" {
			// Verification failed - try to restore policy if we deleted it
			if result.policyWasModified {
				fmt.Printf("  🔁 Channel verification failed - attempting to restore upgrade policy\n")
				if restoreErr := o.restoreUpgradePolicy(ocmClient, cluster, policy); restoreErr != nil {
					result.err = fmt.Errorf("channel verification failed - expected 'eus' but got '%s' (CRITICAL: also failed to restore policy: %v)", newChannel, restoreErr)
				} else {
					result.policyWasRestored = true
					result.err = fmt.Errorf("channel verification failed - expected 'eus' but got '%s' (upgrade policy was restored)", newChannel)
					fmt.Printf("  ✓ Upgrade policy restored after failure\n")
				}
			} else {
				result.err = fmt.Errorf("channel verification failed - expected 'eus' but got '%s'", newChannel)
			}
			return result
		}

		fmt.Printf("  ✓ Channel successfully transitioned to 'eus'\n")
	}

	// Step 5: Restore recurring policy if it existed
	if policy.hasRecurringPolicy {
		if o.dryRun {
			fmt.Printf("  🔍 DRY-RUN: Would restore upgrade policy\n")
		} else {
			fmt.Printf("  🔁 Restoring upgrade policy\n")
			if err := o.restoreUpgradePolicy(ocmClient, cluster, policy); err != nil {
				result.err = fmt.Errorf("failed to restore upgrade policy: %w", err)
				return result
			}
			result.policyWasRestored = true
			fmt.Printf("  ✓ Upgrade policy restored\n")
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
		return &policyDetails{hasRecurringPolicy: false}, nil
	}

	// Get the first policy (there should only be one)
	policy := policies[0]

	return &policyDetails{
		hasRecurringPolicy: true,
		policyID:           policy.ID(),
		schedule:           policy.Schedule(),
		scheduleType:       string(policy.ScheduleType()),
		enableMinor:        policy.EnableMinorVersionUpgrades(),
	}, nil
}

func (o *transitionOptions) deleteUpgradePolicy(ocmClient *sdk.Connection, cluster *v1.Cluster, policyID string) error {
	_, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(cluster.ID()).
		ControlPlane().UpgradePolicies().ControlPlaneUpgradePolicy(policyID).Delete().Send()
	return err
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

func (o *transitionOptions) restoreUpgradePolicy(ocmClient *sdk.Connection, cluster *v1.Cluster, policy *policyDetails) error {
	scheduleType := v1.ScheduleTypeAutomatic
	if policy.scheduleType == "manual" {
		scheduleType = v1.ScheduleTypeManual
	}

	newPolicy, err := v1.NewControlPlaneUpgradePolicy().
		Schedule(policy.schedule).
		ScheduleType(scheduleType).
		UpgradeType(v1.UpgradeTypeControlPlane).
		EnableMinorVersionUpgrades(policy.enableMinor).
		Build()
	if err != nil {
		return fmt.Errorf("failed to build upgrade policy: %w", err)
	}

	_, err = ocmClient.ClustersMgmt().V1().Clusters().Cluster(cluster.ID()).
		ControlPlane().UpgradePolicies().Add().Body(newPolicy).Send()
	return err
}

// loadServiceLogTemplate loads a service log template from either a predefined template name or file path
func loadServiceLogTemplate(templateOrFile string) ([]byte, bool, error) {
	var templateBytes []byte
	var err error
	var usingDefaultTemplate bool

	// Check if it's a template name
	if templateURL, exists := serviceLogTemplates[templateOrFile]; exists {
		// Use predefined template URL
		templateBytes, err = utils.CurlThis(templateURL)
		if err != nil {
			return nil, false, fmt.Errorf("failed to fetch template from %s: %w", templateURL, err)
		}
		usingDefaultTemplate = true
	} else {
		// Treat as file path
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

	// Set cluster-specific fields
	message.ClusterUUID = cluster.ExternalID()
	message.ClusterID = cluster.ID()

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

	if response.Status() != 201 {
		return fmt.Errorf("service log request failed with status: %d", response.Status())
	}

	return nil
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
