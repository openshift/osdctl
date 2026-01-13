package forceupgrade

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

// forceUpgradeOptions contains all options for the force upgrade command
type forceUpgradeOptions struct {
	clusterID          string
	clustersFile       string
	targetYStream      string
	nextRunMinutes     int
	dryRun             bool
	serviceLogTemplate string

	// Parsed cluster IDs (populated during validation)
	clusterIDs []string
}

// Service log template mappings
var serviceLogTemplates = map[string]string{
	"end-of-support": "https://raw.githubusercontent.com/openshift/managed-notifications/refs/heads/master/hcp/end_of_support_force_upgrade.json",
	// Future templates can be added here:
	// "critical-fix": "https://raw.githubusercontent.com/openshift/managed-notifications/refs/heads/master/hcp/critical_fix.json",
	// "security-fix": "https://raw.githubusercontent.com/openshift/managed-notifications/refs/heads/master/hcp/security_fix.json",
}

// Regular expression for valid cluster IDs - alphanumeric characters and hyphens only
var validClusterIDRegex = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

func newCmdForceUpgrade() *cobra.Command {
	opts := &forceUpgradeOptions{}

	cmd := &cobra.Command{
		Use:   "force-upgrade",
		Short: "Schedule forced control plane upgrade for HCP clusters (Requires ForceUpgrader permissions)",
		Long: `Schedule forced control plane upgrades for ROSA HCP clusters. This command skips all validation checks
(critical alerts, cluster conditions, node pool checks, and version gate agreements).

⚠️ REQUIRES ForceUpgrader PERMISSIONS ⚠️

This command can target clusters in two ways:
- Single cluster: --cluster-id <ID>
- Multiple clusters from file: --clusters-file <file.json>

UPGRADE BEHAVIOR:
The command explicitly upgrades clusters to the LATEST Z-STREAM version of the specified Y-stream.
This serves two purposes:
1. Force upgrades to latest z-stream of the SAME y-stream for critical bug fixes
2. Force upgrades to latest z-stream of a SUBSEQUENT y-stream when current y-stream goes out of support

Example: --target-y 4.15 will upgrade to the latest available 4.15.z version (e.g., 4.15.32).`,
		Example: `  # Force upgrade without service log
  osdctl hcp force-upgrade -C cluster123 --target-y 4.15

  # Force upgrade with end-of-support service log
  osdctl hcp force-upgrade -C cluster123 --target-y 4.16 --send-service-log end-of-support

  # Multiple clusters from file with end-of-support service log
  osdctl hcp force-upgrade --clusters-file clusters.json --target-y 4.16 --send-service-log end-of-support

  # Force upgrade with custom service log template file
  osdctl hcp force-upgrade -C cluster123 --target-y 4.15 --send-service-log /path/to/custom-template.json

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

	// Upgrade configuration flags
	cmd.Flags().StringVar(&opts.targetYStream, "target-y", "", "Target Y-stream version (e.g., 4.15) - will upgrade to the LATEST Z-stream of this Y-stream")
	cmd.Flags().IntVar(&opts.nextRunMinutes, "next-run-minutes", 10, "Offset in minutes for scheduling upgrade (minimum 6 for the scheduling to take place)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Simulate the upgrade without making any changes")

	// Service log flags
	cmd.Flags().StringVar(&opts.serviceLogTemplate, "send-service-log", "", "Send service log notification after scheduling upgrade. Specify template name (e.g., 'end-of-support') or file path (e.g., '/path/to/template.json')")

	// Mark required flags
	_ = cmd.MarkFlagRequired("target-y")

	return cmd
}

func (o *forceUpgradeOptions) validate() error {
	// Exactly one cluster targeting method must be provided
	if o.clusterID == "" && o.clustersFile == "" {
		return fmt.Errorf("no cluster identifier has been found, please specify either --cluster-id or --clusters-file")
	}

	if o.clusterID != "" && o.clustersFile != "" {
		return fmt.Errorf("cannot specify both --cluster-id and --clusters-file, choose one")
	}

	if o.nextRunMinutes < 6 {
		return fmt.Errorf("next-run-minutes must be at least 6 minutes")
	}

	// Service log validation
	if o.serviceLogTemplate != "" {
		// Check if it's a template name
		if _, exists := serviceLogTemplates[o.serviceLogTemplate]; !exists {
			// If not a template name, check if it's a valid file path
			if _, err := os.Stat(o.serviceLogTemplate); os.IsNotExist(err) {
				var validTemplates []string
				for template := range serviceLogTemplates {
					validTemplates = append(validTemplates, template)
				}
				return fmt.Errorf("service log value '%s' is neither a valid template name %v nor an existing file", o.serviceLogTemplate, validTemplates)
			}
		}
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
		// Force upgrade requires at least one cluster
		if len(clusterIDs) == 0 {
			return fmt.Errorf("clusters file contains no cluster IDs - the 'clusters' array is empty")
		}
		o.clusterIDs = clusterIDs
	} else {
		o.clusterIDs = []string{o.clusterID}
	}

	return nil
}

func (o *forceUpgradeOptions) Run() error {
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

	// Display cluster list and service log preview before processing
	if err := o.printPreProcessingSummary(clusters); err != nil {
		return fmt.Errorf("failed to display pre-processing summary: %w", err)
	}

	// Ask for confirmation before proceeding (unless in dry-run mode)
	if !o.dryRun {
		if !ocmutils.ConfirmPrompt() {
			fmt.Println("Force upgrade operation cancelled.")
			return nil
		}
		fmt.Println()
	}

	var successful, failed []string
	var serviceLogSuccessful, serviceLogFailed []string

	for i, cluster := range clusters {
		fmt.Printf("\n[%d/%d] Processing cluster: %s (%s)\n", i+1, len(clusters), cluster.ID(), cluster.Name())

		targetVersion, err := o.processCluster(ocmClient, cluster)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %s", cluster.ExternalID(), err.Error()))
			fmt.Printf("  ⚠️  Failed to create upgrade policy: %v\n", err)
		} else {
			successful = append(successful, cluster.ExternalID())

			if o.serviceLogTemplate != "" {
				if o.dryRun {
					fmt.Printf("  📧 DRY-RUN: Would send service log notification\n")
					serviceLogSuccessful = append(serviceLogSuccessful, cluster.ExternalID())
					continue
				}

				if err := sendUpgradeServiceLog(ocmClient, cluster, o.serviceLogTemplate, targetVersion); err != nil {
					serviceLogFailed = append(serviceLogFailed, fmt.Sprintf("%s: %s", cluster.ExternalID(), err.Error()))
					fmt.Printf("  ⚠️  Failed to send service log: %v\n", err)
				} else {
					serviceLogSuccessful = append(serviceLogSuccessful, cluster.ExternalID())
					fmt.Printf("  📧 Service log notification sent successfully\n")
				}
			}
		}
	}

	o.printSummary(successful, failed, serviceLogSuccessful, serviceLogFailed)
	return nil
}

func (o *forceUpgradeOptions) getClusters(ocmClient *sdk.Connection) ([]*v1.Cluster, error) {
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

func (o *forceUpgradeOptions) processCluster(ocmClient *sdk.Connection, cluster *v1.Cluster) (string, error) {
	// Some sanity checking - we should only ever be upgrading ROSA HCP clusters.
	if !cluster.Hypershift().Enabled() {
		return "", fmt.Errorf("force upgrading is only allowed on ROSA HCP clusters")
	}

	// Check cluster state
	if cluster.State() != v1.ClusterStateReady {
		return "", fmt.Errorf("cluster is not ready (current state: %s)", cluster.State())
	}

	// Check for available upgrades
	if len(cluster.Version().AvailableUpgrades()) == 0 {
		return "", fmt.Errorf("no available upgrades path")
	}

	// Check for existing upgrade policies
	policiesResponse, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(cluster.ID()).
		ControlPlane().UpgradePolicies().List().Send()
	if err != nil {
		return "", fmt.Errorf("failed to list existing upgrade policies: %w", err)
	}

	var automaticPolicyIDs []string
	for _, policy := range policiesResponse.Items().Slice() {
		if policy.ScheduleType() == v1.ScheduleTypeAutomatic {
			automaticPolicyIDs = append(automaticPolicyIDs, policy.ID())
		} else {
			return "", fmt.Errorf("existing manual upgrade policy found: target version %s scheduled at %s",
				policy.Version(), policy.NextRun().Format(time.RFC3339))
		}
	}

	// Find target version
	targetVersion, err := o.determineTargetVersion(cluster.Version().AvailableUpgrades())
	if err != nil {
		return "", fmt.Errorf("failed to determine target version: %w", err)
	}

	if targetVersion == "" {
		return "", fmt.Errorf("no valid upgrade version found for Y-stream '%s'", o.targetYStream)
	}

	scheduleTime := time.Now().UTC().Add(time.Duration(o.nextRunMinutes) * time.Minute)

	if o.dryRun {
		fmt.Printf("  🔍 DRY RUN: Would schedule force upgrade to %s at %s\n",
			targetVersion, scheduleTime.Format(time.RFC3339))
		return targetVersion, nil
	}

	// Delete automatic Z-stream upgrade policies
	for _, id := range automaticPolicyIDs {
		fmt.Printf("  🗑️  Deleting automatic Z-stream upgrade policy (ID: %s) ahead of scheduling manual upgrade\n", id)
		_, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(cluster.ID()).
			ControlPlane().UpgradePolicies().ControlPlaneUpgradePolicy(id).Delete().Send()
		if err != nil {
			return "", fmt.Errorf("failed to delete automatic upgrade policy (ID: %s): %w", id, err)
		}
	}

	policy, err := v1.NewControlPlaneUpgradePolicy().
		Version(targetVersion).
		ScheduleType(v1.ScheduleTypeManual).
		UpgradeType(v1.UpgradeTypeControlPlaneCVE). // UpgradeTypeControlPlaneCVE skips pre-flights such as OCM version gates, nodepool version constraints, cluster conditions, actively firing alerts (see OCM-DDR-0204)
		NextRun(scheduleTime).
		Build()
	if err != nil {
		return "", fmt.Errorf("failed to build upgrade policy: %w", err)
	}

	_, err = ocmClient.ClustersMgmt().V1().Clusters().Cluster(cluster.ID()).
		ControlPlane().UpgradePolicies().Add().Body(policy).Send()
	if err != nil {
		return "", fmt.Errorf("failed to create upgrade policy: %w", err)
	}

	fmt.Printf("  ✅ Scheduled force upgrade to version %s at %s\n",
		targetVersion, scheduleTime.Format(time.RFC3339))

	return targetVersion, nil
}

func (o *forceUpgradeOptions) determineTargetVersion(availableUpgrades []string) (string, error) {
	var matchingVersions []*semver.Version

	for _, upgrade := range availableUpgrades {
		version, err := semver.NewVersion(upgrade)
		if err != nil {
			fmt.Printf("  ⚠️  Skipping invalid version: %s\n", upgrade)
			continue
		}

		// Extract Y-stream (major.minor) and compare
		upgradeYStream := fmt.Sprintf("%d.%d", version.Major(), version.Minor())
		if upgradeYStream == o.targetYStream {
			matchingVersions = append(matchingVersions, version)
		}
	}

	if len(matchingVersions) == 0 {
		return "", nil
	}

	// Sort and return the highest version
	var latest *semver.Version
	for _, version := range matchingVersions {
		if latest == nil || version.GreaterThan(latest) {
			latest = version
		}
	}

	return latest.Original(), nil
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

func sendUpgradeServiceLog(ocmClient *sdk.Connection, cluster *v1.Cluster, templateOrFile, targetVersion string) error {
	templateBytes, usingDefaultTemplate, err := loadServiceLogTemplate(templateOrFile)
	if err != nil {
		return err
	}

	if usingDefaultTemplate {
		fmt.Printf("  📄 Using service log template: %s\n", templateOrFile)
	} else {
		fmt.Printf("  📄 Using custom service log template file: %s\n", templateOrFile)
	}

	var message servicelog.Message
	if err := json.Unmarshal(templateBytes, &message); err != nil {
		return fmt.Errorf("failed to parse service log template: %w", err)
	}

	// Set cluster-specific fields
	message.ClusterUUID = cluster.ExternalID()
	message.ClusterID = cluster.ID()

	// Only replace VERSION parameter if using the default template
	if usingDefaultTemplate {
		message.ReplaceWithFlag("${VERSION}", targetVersion)
	}

	// Validate that all required parameters were replaced
	if leftoverParams, found := message.FindLeftovers(); found {
		if usingDefaultTemplate {
			return fmt.Errorf("default template contains unresolved parameters: %v. This should not happen", leftoverParams)
		} else {
			return fmt.Errorf("custom template contains unresolved parameters: %v. Please ensure all parameters are defined in your template", leftoverParams)
		}
	}

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

func (o *forceUpgradeOptions) printSummary(successful, failed, serviceLogSuccessful, serviceLogFailed []string) {
	fmt.Print("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Print("FORCE UPGRADE SUMMARY\n")
	fmt.Print(strings.Repeat("=", 60) + "\n")

	total := len(successful) + len(failed)
	fmt.Printf("Total clusters processed: %d\n", total)
	fmt.Printf("Successfully scheduled: %d\n", len(successful))
	fmt.Printf("Failed: %d\n", len(failed))

	if len(failed) > 0 {
		fmt.Printf("\n⚠️ Failed to create upgrade policies for the following clusters (please follow-up manually):\n")
		for _, entry := range failed {
			fmt.Printf("  - %s\n", entry)
		}
	}

	if o.serviceLogTemplate != "" {
		fmt.Printf("\n📧 SERVICE LOG SUMMARY:\n")
		fmt.Printf("Successfully sent: %d\n", len(serviceLogSuccessful))
		fmt.Printf("Failed to send: %d\n", len(serviceLogFailed))

		if len(serviceLogFailed) > 0 {
			fmt.Printf("\n⚠️  Failed to send service logs for the following clusters (please follow-up manually):\n")
			for _, entry := range serviceLogFailed {
				fmt.Printf("  - %s\n", entry)
			}
		}
	}

	fmt.Print(strings.Repeat("=", 60) + "\n")
}

// printPreProcessingSummary displays the clusters and service log template before processing
func (o *forceUpgradeOptions) printPreProcessingSummary(clusters []*v1.Cluster) error {
	fmt.Print("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Print("PRE-PROCESSING SUMMARY\n")

	fmt.Print(strings.Repeat("=", 60) + "\n")

	fmt.Printf("\nClusters to be upgraded (Target Y-stream: %s):\n", o.targetYStream)
	for i, cluster := range clusters {
		fmt.Printf("%2d. %s (%s) - %s - %s\n",
			i+1,
			cluster.ExternalID(),
			cluster.Name(),
			cluster.State(),
			cluster.OpenshiftVersion(),
		)
	}

	// Display service log template if service logs are enabled
	if o.serviceLogTemplate != "" {
		fmt.Printf("\nService Log to be sent after scheduling upgrades:\n")

		templateBytes, usingDefaultTemplate, err := loadServiceLogTemplate(o.serviceLogTemplate)
		if err != nil {
			return err
		}

		// Pretty print the JSON template
		var jsonData any
		if err := json.Unmarshal(templateBytes, &jsonData); err != nil {
			return fmt.Errorf("failed to parse template: %w", err)
		}

		prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format template JSON: %w", err)
		}
		fmt.Printf("Template:\n%s\n", string(prettyJSON))

		if usingDefaultTemplate {
			fmt.Printf("\nNote: The ${VERSION} parameter will be replaced with the target upgrade version for each cluster.\n")
		} else {
			fmt.Printf("\nNote: Custom template files are used as-is without automatic parameter replacement.\n")
		}
	}

	fmt.Print(strings.Repeat("=", 60) + "\n")

	return nil
}
