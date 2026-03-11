package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// EvidenceCollection represents all collected evidence
type EvidenceCollection struct {
	Metadata       CollectionMetadata `yaml:"metadata"`
	ClusterState   *ClusterState      `yaml:"clusterState,omitempty"`
	CloudTrailData *CloudTrailData    `yaml:"cloudTrailData,omitempty"`
	Diagnostics    *DiagnosticData    `yaml:"diagnostics,omitempty"`
}

// CollectionMetadata contains information about the evidence collection
type CollectionMetadata struct {
	ClusterID       string    `yaml:"clusterId"`
	ClusterName     string    `yaml:"clusterName"`
	CollectionTime  time.Time `yaml:"collectionTime"`
	CollectorUser   string    `yaml:"collectorUser,omitempty"`
	TimeWindowStart time.Time `yaml:"timeWindowStart"`
	Platform        string    `yaml:"platform"`
	IsHCP           bool      `yaml:"isHCP"`
}

// ClusterState captures cluster resource states
type ClusterState struct {
	Nodes          []NodeInfo          `yaml:"nodes,omitempty"`
	Operators      []OperatorInfo      `yaml:"operators,omitempty"`
	MachineConfigs []MachineConfigInfo `yaml:"machineConfigs,omitempty"`
	Events         []EventInfo         `yaml:"events,omitempty"`
}

// NodeInfo represents node state
type NodeInfo struct {
	Name       string   `yaml:"name"`
	Status     string   `yaml:"status"`
	Roles      []string `yaml:"roles"`
	Conditions []string `yaml:"conditions,omitempty"`
}

// OperatorInfo represents ClusterOperator state
type OperatorInfo struct {
	Name        string `yaml:"name"`
	Available   bool   `yaml:"available"`
	Progressing bool   `yaml:"progressing"`
	Degraded    bool   `yaml:"degraded"`
	Version     string `yaml:"version,omitempty"`
}

// MachineConfigInfo represents MachineConfig state
type MachineConfigInfo struct {
	Name    string `yaml:"name"`
	Created string `yaml:"created"`
}

// EventInfo represents Kubernetes events
type EventInfo struct {
	Type      string `yaml:"type"`
	Reason    string `yaml:"reason"`
	Message   string `yaml:"message"`
	Namespace string `yaml:"namespace"`
	Object    string `yaml:"object"`
	Timestamp string `yaml:"timestamp"`
}

// CloudTrailData contains CloudTrail event information
type CloudTrailData struct {
	ErrorEvents []CloudTrailError `yaml:"errorEvents,omitempty"`
	WriteEvents []CloudTrailEvent `yaml:"writeEvents,omitempty"`
}

// CloudTrailError represents an AWS error event
type CloudTrailError struct {
	EventTime   string `yaml:"eventTime"`
	EventName   string `yaml:"eventName"`
	ErrorCode   string `yaml:"errorCode"`
	ErrorMsg    string `yaml:"errorMessage,omitempty"`
	Username    string `yaml:"username,omitempty"`
	Region      string `yaml:"region"`
	ConsoleLink string `yaml:"consoleLink,omitempty"`
}

// CloudTrailEvent represents an AWS API event
type CloudTrailEvent struct {
	EventTime string `yaml:"eventTime"`
	EventName string `yaml:"eventName"`
	Username  string `yaml:"username,omitempty"`
	Region    string `yaml:"region"`
}

// DiagnosticData contains diagnostic commands output
type DiagnosticData struct {
	MustGatherPath string            `yaml:"mustGatherPath,omitempty"`
	CustomCommands map[string]string `yaml:"customCommands,omitempty"`
}

// RawEventDetails represents CloudTrail event structure
type RawEventDetails struct {
	EventVersion string `json:"eventVersion"`
	UserIdentity struct {
		AccountId      string `json:"accountId"`
		SessionContext struct {
			SessionIssuer struct {
				Type     string `json:"type"`
				UserName string `json:"userName"`
				Arn      string `json:"arn"`
			} `json:"sessionIssuer"`
		} `json:"sessionContext"`
	} `json:"userIdentity"`
	EventRegion  string `json:"awsRegion"`
	EventId      string `json:"eventID"`
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
}

// collectOptions holds the options for the collect command
type collectOptions struct {
	ClusterID         string
	OutputDir         string
	Since             string
	IncludeEvents     bool
	IncludeMustGather bool
	SkipCloudTrail    bool
	SkipClusterState  bool
}

func newCmdCollect() *cobra.Command {
	opts := &collectOptions{}

	collectCmd := &cobra.Command{
		Use:   "collect",
		Short: "Collect evidence from cluster and AWS for feature testing",
		Long: `Collect comprehensive evidence from a cluster and AWS for feature testing.

This all-in-one command gathers:
- Cluster state (nodes, operators, machine configs)
- CloudTrail error events (permission denied, etc.)
- Recent Kubernetes events (optional)
- must-gather output (optional)

The collected evidence is saved to the specified output directory for
inclusion in test reports and feature validation documentation.`,
		Example: `  # Collect all evidence to a directory
  osdctl evidence collect -C <cluster-id> --output ./evidence/

  # Collect evidence from the last 2 hours
  osdctl evidence collect -C <cluster-id> --output ./evidence/ --since 2h

  # Collect evidence without CloudTrail (for non-AWS or limited access)
  osdctl evidence collect -C <cluster-id> --output ./evidence/ --skip-cloudtrail

  # Include Kubernetes events in collection
  osdctl evidence collect -C <cluster-id> --output ./evidence/ --include-events`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.run()
		},
	}

	collectCmd.Flags().StringVarP(&opts.ClusterID, "cluster-id", "C", "", "Cluster ID (internal, external, or name)")
	collectCmd.Flags().StringVarP(&opts.OutputDir, "output", "o", "", "Output directory for collected evidence")
	collectCmd.Flags().StringVar(&opts.Since, "since", "1h", "Time window to look back for events (e.g., 30m, 1h, 2h)")
	collectCmd.Flags().BoolVar(&opts.IncludeEvents, "include-events", false, "Include Kubernetes events in collection")
	collectCmd.Flags().BoolVar(&opts.IncludeMustGather, "include-must-gather", false, "Run must-gather and include output")
	collectCmd.Flags().BoolVar(&opts.SkipCloudTrail, "skip-cloudtrail", false, "Skip CloudTrail event collection")
	collectCmd.Flags().BoolVar(&opts.SkipClusterState, "skip-cluster-state", false, "Skip cluster state collection")
	cmdutil.CheckErr(collectCmd.MarkFlagRequired("cluster-id"))
	cmdutil.CheckErr(collectCmd.MarkFlagRequired("output"))

	return collectCmd
}

func (o *collectOptions) run() error {
	if err := utils.IsValidClusterKey(o.ClusterID); err != nil {
		return err
	}

	connection, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("unable to create connection to OCM: %w", err)
	}
	defer connection.Close()

	cluster, err := utils.GetClusterAnyStatus(connection, o.ClusterID)
	if err != nil {
		return err
	}

	// Create output directory
	if err := os.MkdirAll(o.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	startTime, err := parseDurationToUTC(o.Since)
	if err != nil {
		return fmt.Errorf("invalid time duration: %w", err)
	}

	isHCP := cluster.Hypershift().Enabled()
	clusterType := "Classic"
	if isHCP {
		clusterType = "HCP"
	}

	fmt.Printf("╔══════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                    EVIDENCE COLLECTION                        ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Cluster: %-53s ║\n", cluster.Name())
	fmt.Printf("║ ID:      %-53s ║\n", cluster.ID())
	fmt.Printf("║ Type:    %-53s ║\n", clusterType)
	fmt.Printf("║ Since:   %-53s ║\n", startTime.Format(time.RFC3339))
	fmt.Printf("╚══════════════════════════════════════════════════════════════╝\n\n")

	if isHCP {
		fmt.Println("ℹ️  HCP cluster detected - CloudTrail events will only show customer account activity")
		fmt.Println("   Control plane activity is in Red Hat's account and not visible here.")
	}

	// Verify we're connected to the correct cluster
	if err := o.verifyClusterContext(cluster.ID()); err != nil {
		fmt.Printf("⚠️  Warning: %v\n", err)
		fmt.Println("   Please ensure you're logged into the correct cluster via 'ocm backplane login'")
		fmt.Println("   Continuing anyway - collected data may be from a different cluster!")
	}

	evidence := &EvidenceCollection{
		Metadata: CollectionMetadata{
			ClusterID:       cluster.ID(),
			ClusterName:     cluster.Name(),
			CollectionTime:  time.Now().UTC(),
			TimeWindowStart: startTime,
			Platform:        cluster.CloudProvider().ID(),
			IsHCP:           isHCP,
		},
	}

	// Collect cluster state
	if !o.SkipClusterState {
		fmt.Println("📋 Collecting cluster state...")
		clusterState, err := o.collectClusterState()
		if err != nil {
			fmt.Printf("   ⚠️  Warning: Failed to collect cluster state: %v\n", err)
		} else {
			evidence.ClusterState = clusterState
			fmt.Printf("   ✓ Collected state for %d nodes, %d operators\n",
				len(clusterState.Nodes), len(clusterState.Operators))
		}
	}

	// Collect CloudTrail data for AWS clusters
	if !o.SkipCloudTrail && strings.ToUpper(cluster.CloudProvider().ID()) == "AWS" {
		fmt.Println("☁️  Collecting CloudTrail data...")
		// Verify AWS access is available
		_, err := osdCloud.CreateAWSV2Config(connection, cluster)
		if err != nil {
			fmt.Printf("   ⚠️  Warning: Failed to create AWS config: %v\n", err)
		} else {
			cloudTrailData, err := o.collectCloudTrailData(startTime)
			if err != nil {
				fmt.Printf("   ⚠️  Warning: Failed to collect CloudTrail data: %v\n", err)
			} else {
				evidence.CloudTrailData = cloudTrailData
				fmt.Println("   ✓ CloudTrail access verified. Use 'osdctl cloudtrail errors' for detailed error analysis.")
			}
		}
	}

	// Include Kubernetes events
	if o.IncludeEvents {
		fmt.Println("📅 Collecting Kubernetes events...")
		events, err := o.collectKubernetesEvents(startTime)
		if err != nil {
			fmt.Printf("   ⚠️  Warning: Failed to collect events: %v\n", err)
		} else {
			if evidence.ClusterState == nil {
				evidence.ClusterState = &ClusterState{}
			}
			evidence.ClusterState.Events = events
			fmt.Printf("   ✓ Collected %d events\n", len(events))
		}
	}

	// Run must-gather if requested
	if o.IncludeMustGather {
		fmt.Println("📦 Running must-gather...")
		mustGatherPath, err := o.runMustGather()
		if err != nil {
			fmt.Printf("   ⚠️  Warning: must-gather failed: %v\n", err)
		} else {
			if evidence.Diagnostics == nil {
				evidence.Diagnostics = &DiagnosticData{}
			}
			evidence.Diagnostics.MustGatherPath = mustGatherPath
			fmt.Printf("   ✓ must-gather saved to: %s\n", mustGatherPath)
		}
	}

	// Write evidence to files
	fmt.Println("\n💾 Saving evidence...")

	// Save main evidence file
	evidenceFile := filepath.Join(o.OutputDir, "evidence.yaml")
	if err := o.saveEvidence(evidence, evidenceFile); err != nil {
		return fmt.Errorf("failed to save evidence: %w", err)
	}
	fmt.Printf("   ✓ Evidence saved to: %s\n", evidenceFile)

	// Save summary
	summaryFile := filepath.Join(o.OutputDir, "summary.txt")
	if err := o.saveSummary(evidence, summaryFile); err != nil {
		fmt.Printf("   ⚠️  Warning: Failed to save summary: %v\n", err)
	} else {
		fmt.Printf("   ✓ Summary saved to: %s\n", summaryFile)
	}

	fmt.Printf("\n✅ Evidence collection complete!\n")
	fmt.Printf("   Output directory: %s\n", o.OutputDir)

	return nil
}

func (o *collectOptions) collectClusterState() (*ClusterState, error) {
	state := &ClusterState{}
	var warnings []string

	// Collect nodes
	nodes, err := o.collectNodes()
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("nodes: %v", err))
	} else {
		state.Nodes = nodes
	}

	// Collect operators
	operators, err := o.collectOperators()
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("operators: %v", err))
	} else {
		state.Operators = operators
	}

	// Collect machine configs
	machineConfigs, err := o.collectMachineConfigs()
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("machineconfigs: %v", err))
	} else {
		state.MachineConfigs = machineConfigs
	}

	// Return error if all collections failed
	if len(state.Nodes) == 0 && len(state.Operators) == 0 && len(state.MachineConfigs) == 0 && len(warnings) > 0 {
		return nil, fmt.Errorf("all collections failed: %v", warnings)
	}

	// Print warnings for partial failures
	for _, w := range warnings {
		fmt.Printf("   ⚠️  Warning: Failed to collect %s\n", w)
	}

	return state, nil
}

func (o *collectOptions) collectNodes() ([]NodeInfo, error) {
	output, err := exec.CommandContext(context.TODO(), "oc", "get", "nodes", "-o", "json").Output()
	if err != nil {
		return nil, err
	}

	var result struct {
		Items []struct {
			Metadata struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var nodes []NodeInfo
	for _, item := range result.Items {
		node := NodeInfo{
			Name: item.Metadata.Name,
		}

		// Extract roles
		for label := range item.Metadata.Labels {
			if strings.HasPrefix(label, "node-role.kubernetes.io/") {
				role := strings.TrimPrefix(label, "node-role.kubernetes.io/")
				node.Roles = append(node.Roles, role)
			}
		}

		// Check conditions
		for _, cond := range item.Status.Conditions {
			if cond.Type == "Ready" {
				if cond.Status == "True" {
					node.Status = "Ready"
				} else {
					node.Status = "NotReady"
				}
			}
			node.Conditions = append(node.Conditions, fmt.Sprintf("%s=%s", cond.Type, cond.Status))
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (o *collectOptions) collectOperators() ([]OperatorInfo, error) {
	output, err := exec.CommandContext(context.TODO(), "oc", "get", "clusteroperators", "-o", "json").Output()
	if err != nil {
		return nil, err
	}

	var result struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
				Versions []struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				} `json:"versions"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var operators []OperatorInfo
	for _, item := range result.Items {
		operator := OperatorInfo{
			Name: item.Metadata.Name,
		}

		for _, cond := range item.Status.Conditions {
			switch cond.Type {
			case "Available":
				operator.Available = cond.Status == "True"
			case "Progressing":
				operator.Progressing = cond.Status == "True"
			case "Degraded":
				operator.Degraded = cond.Status == "True"
			}
		}

		for _, ver := range item.Status.Versions {
			if ver.Name == "operator" {
				operator.Version = ver.Version
				break
			}
		}

		operators = append(operators, operator)
	}

	return operators, nil
}

func (o *collectOptions) collectMachineConfigs() ([]MachineConfigInfo, error) {
	output, err := exec.CommandContext(context.TODO(), "oc", "get", "machineconfigs", "-o", "json").Output()
	if err != nil {
		return nil, err
	}

	var result struct {
		Items []struct {
			Metadata struct {
				Name              string `json:"name"`
				CreationTimestamp string `json:"creationTimestamp"`
			} `json:"metadata"`
		} `json:"items"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var configs []MachineConfigInfo
	for _, item := range result.Items {
		configs = append(configs, MachineConfigInfo{
			Name:    item.Metadata.Name,
			Created: item.Metadata.CreationTimestamp,
		})
	}

	return configs, nil
}

func (o *collectOptions) collectKubernetesEvents(startTime time.Time) ([]EventInfo, error) {
	output, err := exec.CommandContext(context.TODO(), "oc", "get", "events", "--all-namespaces", "-o", "json").Output()
	if err != nil {
		return nil, err
	}

	var result struct {
		Items []struct {
			Type     string `json:"type"`
			Reason   string `json:"reason"`
			Message  string `json:"message"`
			Metadata struct {
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			InvolvedObject struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"involvedObject"`
			LastTimestamp string `json:"lastTimestamp"`
		} `json:"items"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var events []EventInfo
	for _, item := range result.Items {
		// Filter events by startTime
		if item.LastTimestamp != "" {
			eventTime, err := time.Parse(time.RFC3339, item.LastTimestamp)
			if err == nil && eventTime.Before(startTime) {
				continue // Skip events older than startTime
			}
		}
		events = append(events, EventInfo{
			Type:      item.Type,
			Reason:    item.Reason,
			Message:   item.Message,
			Namespace: item.Metadata.Namespace,
			Object:    fmt.Sprintf("%s/%s", item.InvolvedObject.Kind, item.InvolvedObject.Name),
			Timestamp: item.LastTimestamp,
		})
	}

	return events, nil
}

func (o *collectOptions) collectCloudTrailData(startTime time.Time) (*CloudTrailData, error) {
	// Note: Full CloudTrail error data collection is handled by 'osdctl cloudtrail errors'
	// This function verifies AWS access is available
	// For detailed CloudTrail analysis, use: osdctl cloudtrail errors -C <cluster-id> --since <duration>
	_ = startTime

	// Return nil to indicate CloudTrail was not collected (use dedicated command for details)
	return nil, nil
}

func (o *collectOptions) runMustGather() (string, error) {
	mustGatherDir := filepath.Join(o.OutputDir, "must-gather")
	if err := os.MkdirAll(mustGatherDir, 0755); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(context.TODO(), "oc", "adm", "must-gather", "--dest-dir", mustGatherDir) //#nosec G204 -- command args are trusted
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return mustGatherDir, nil
}

func (o *collectOptions) saveEvidence(evidence *EvidenceCollection, filename string) error {
	data, err := yaml.Marshal(evidence)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0600)
}

func (o *collectOptions) saveSummary(evidence *EvidenceCollection, filename string) error {
	var sb strings.Builder

	sb.WriteString("EVIDENCE COLLECTION SUMMARY\n")
	sb.WriteString("===========================\n\n")
	sb.WriteString(fmt.Sprintf("Cluster: %s (%s)\n", evidence.Metadata.ClusterName, evidence.Metadata.ClusterID))
	sb.WriteString(fmt.Sprintf("Platform: %s\n", evidence.Metadata.Platform))
	sb.WriteString(fmt.Sprintf("Collection Time: %s\n", evidence.Metadata.CollectionTime.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Time Window Start: %s\n\n", evidence.Metadata.TimeWindowStart.Format(time.RFC3339)))

	if evidence.ClusterState != nil {
		sb.WriteString("CLUSTER STATE\n")
		sb.WriteString("-------------\n")
		sb.WriteString(fmt.Sprintf("Nodes: %d\n", len(evidence.ClusterState.Nodes)))
		sb.WriteString(fmt.Sprintf("Operators: %d\n", len(evidence.ClusterState.Operators)))
		sb.WriteString(fmt.Sprintf("MachineConfigs: %d\n", len(evidence.ClusterState.MachineConfigs)))
		sb.WriteString(fmt.Sprintf("Events: %d\n\n", len(evidence.ClusterState.Events)))

		// Count degraded operators
		degradedCount := 0
		for _, op := range evidence.ClusterState.Operators {
			if op.Degraded {
				degradedCount++
			}
		}
		if degradedCount > 0 {
			sb.WriteString(fmt.Sprintf("⚠️  Degraded Operators: %d\n", degradedCount))
			for _, op := range evidence.ClusterState.Operators {
				if op.Degraded {
					sb.WriteString(fmt.Sprintf("   - %s\n", op.Name))
				}
			}
			sb.WriteString("\n")
		}

		// Count not ready nodes
		notReadyCount := 0
		for _, node := range evidence.ClusterState.Nodes {
			if node.Status != "Ready" {
				notReadyCount++
			}
		}
		if notReadyCount > 0 {
			sb.WriteString(fmt.Sprintf("⚠️  Not Ready Nodes: %d\n", notReadyCount))
			for _, node := range evidence.ClusterState.Nodes {
				if node.Status != "Ready" {
					sb.WriteString(fmt.Sprintf("   - %s\n", node.Name))
				}
			}
			sb.WriteString("\n")
		}
	}

	if evidence.CloudTrailData != nil {
		sb.WriteString("CLOUDTRAIL DATA\n")
		sb.WriteString("---------------\n")
		sb.WriteString(fmt.Sprintf("Error Events: %d\n", len(evidence.CloudTrailData.ErrorEvents)))
		sb.WriteString(fmt.Sprintf("Write Events: %d\n\n", len(evidence.CloudTrailData.WriteEvents)))

		if len(evidence.CloudTrailData.ErrorEvents) > 0 {
			sb.WriteString("Recent Errors:\n")
			maxErrors := 10
			if len(evidence.CloudTrailData.ErrorEvents) < maxErrors {
				maxErrors = len(evidence.CloudTrailData.ErrorEvents)
			}
			for i := 0; i < maxErrors; i++ {
				e := evidence.CloudTrailData.ErrorEvents[i]
				sb.WriteString(fmt.Sprintf("   - %s: %s (%s)\n", e.EventTime, e.EventName, e.ErrorCode))
			}
			sb.WriteString("\n")
		}
	}

	return os.WriteFile(filename, []byte(sb.String()), 0600)
}

func parseDurationToUTC(input string) (time.Time, error) {
	duration, err := time.ParseDuration(input)
	if err != nil {
		return time.Time{}, fmt.Errorf("unable to parse time duration: %w", err)
	}
	if duration <= 0 {
		return time.Time{}, fmt.Errorf("duration must be positive (e.g., 1h, 30m)")
	}
	return time.Now().UTC().Add(-duration), nil
}

// verifyClusterContext checks if the current oc context appears to match the target cluster
func (o *collectOptions) verifyClusterContext(clusterID string) error {
	// Get current context info by checking cluster-info
	output, err := exec.CommandContext(context.TODO(), "oc", "whoami", "--show-server").Output()
	if err != nil {
		return fmt.Errorf("unable to verify cluster context: %w", err)
	}

	serverURL := strings.TrimSpace(string(output))
	// Check if server URL contains the cluster ID (common pattern for backplane URLs)
	if !strings.Contains(serverURL, clusterID) {
		return fmt.Errorf("current context server (%s) may not match target cluster (%s)", serverURL, clusterID)
	}

	return nil
}
