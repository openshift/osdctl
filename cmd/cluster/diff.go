package cluster

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// DiffResult represents the comparison between two snapshots
type DiffResult struct {
	BeforeSnapshot   string                    `yaml:"beforeSnapshot" json:"beforeSnapshot"`
	AfterSnapshot    string                    `yaml:"afterSnapshot" json:"afterSnapshot"`
	Summary          DiffSummary               `yaml:"summary" json:"summary"`
	NodeChanges      []NodeDiff                `yaml:"nodeChanges,omitempty" json:"nodeChanges,omitempty"`
	OperatorChanges  []OperatorDiff            `yaml:"operatorChanges,omitempty" json:"operatorChanges,omitempty"`
	NamespaceChanges []NamespaceDiff           `yaml:"namespaceChanges,omitempty" json:"namespaceChanges,omitempty"`
	ResourceChanges  map[string][]ResourceDiff `yaml:"resourceChanges,omitempty" json:"resourceChanges,omitempty"`
}

// DiffSummary provides high-level change counts
type DiffSummary struct {
	TotalChanges      int `yaml:"totalChanges" json:"totalChanges"`
	NodesChanged      int `yaml:"nodesChanged" json:"nodesChanged"`
	OperatorsChanged  int `yaml:"operatorsChanged" json:"operatorsChanged"`
	NamespacesChanged int `yaml:"namespacesChanged" json:"namespacesChanged"`
	ResourcesChanged  int `yaml:"resourcesChanged" json:"resourcesChanged"`
}

// NodeDiff represents changes to a node
type NodeDiff struct {
	Name       string `yaml:"name" json:"name"`
	ChangeType string `yaml:"changeType" json:"changeType"` // added, removed, modified
	Before     string `yaml:"before,omitempty" json:"before,omitempty"`
	After      string `yaml:"after,omitempty" json:"after,omitempty"`
}

// OperatorDiff represents changes to a ClusterOperator
type OperatorDiff struct {
	Name       string `yaml:"name" json:"name"`
	ChangeType string `yaml:"changeType" json:"changeType"`
	Field      string `yaml:"field,omitempty" json:"field,omitempty"`
	Before     string `yaml:"before,omitempty" json:"before,omitempty"`
	After      string `yaml:"after,omitempty" json:"after,omitempty"`
}

// NamespaceDiff represents changes to a namespace
type NamespaceDiff struct {
	Name       string `yaml:"name" json:"name"`
	ChangeType string `yaml:"changeType" json:"changeType"`
	Before     string `yaml:"before,omitempty" json:"before,omitempty"`
	After      string `yaml:"after,omitempty" json:"after,omitempty"`
}

// ResourceDiff represents changes to a resource
type ResourceDiff struct {
	Name       string `yaml:"name" json:"name"`
	Namespace  string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	ChangeType string `yaml:"changeType" json:"changeType"`
	Before     string `yaml:"before,omitempty" json:"before,omitempty"`
	After      string `yaml:"after,omitempty" json:"after,omitempty"`
}

// diffOptions holds the options for the diff command
type diffOptions struct {
	BeforeFile string
	AfterFile  string
	OutputJSON bool
}

func newCmdDiff() *cobra.Command {
	opts := &diffOptions{}

	diffCmd := &cobra.Command{
		Use:   "diff <before.yaml> <after.yaml>",
		Short: "Compare two cluster snapshots to identify changes",
		Long: `Compare two cluster snapshots to identify changes.

This command compares two snapshot files created by 'osdctl cluster snapshot'
and reports the differences. This is useful for understanding what changed
in a cluster during feature testing or validation.

Changes are categorized as:
- added: Resource exists in after but not in before
- removed: Resource exists in before but not in after  
- modified: Resource exists in both but with different values`,
		Example: `  # Compare two snapshots
  osdctl cluster diff before.yaml after.yaml

  # Compare snapshots with JSON output
  osdctl cluster diff before.yaml after.yaml --json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BeforeFile = args[0]
			opts.AfterFile = args[1]
			return opts.run()
		},
	}

	diffCmd.Flags().BoolVar(&opts.OutputJSON, "json", false, "Output diff in JSON format")

	return diffCmd
}

func (o *diffOptions) run() error {
	// Load before snapshot
	beforeSnapshot, err := loadSnapshot(o.BeforeFile)
	if err != nil {
		return fmt.Errorf("failed to load before snapshot: %w", err)
	}

	// Load after snapshot
	afterSnapshot, err := loadSnapshot(o.AfterFile)
	if err != nil {
		return fmt.Errorf("failed to load after snapshot: %w", err)
	}

	// Validate snapshots are from the same cluster
	if beforeSnapshot.Metadata.ClusterID != afterSnapshot.Metadata.ClusterID {
		return fmt.Errorf("snapshots are from different clusters: %s (%s) vs %s (%s)",
			beforeSnapshot.Metadata.ClusterName, beforeSnapshot.Metadata.ClusterID,
			afterSnapshot.Metadata.ClusterName, afterSnapshot.Metadata.ClusterID)
	}

	// Compare snapshots
	result := compareSnapshots(beforeSnapshot, afterSnapshot, o.BeforeFile, o.AfterFile)

	// Print results
	return o.printDiff(result)
}

func loadSnapshot(filename string) (*ClusterSnapshot, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var snapshot ClusterSnapshot
	if err := yaml.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}

	return &snapshot, nil
}

func compareSnapshots(before, after *ClusterSnapshot, beforeFile, afterFile string) *DiffResult {
	result := &DiffResult{
		BeforeSnapshot:  beforeFile,
		AfterSnapshot:   afterFile,
		ResourceChanges: make(map[string][]ResourceDiff),
	}

	// Warn about capture errors that could cause false diffs
	warnCaptureErrors(before, "before", beforeFile)
	warnCaptureErrors(after, "after", afterFile)

	// Compare nodes
	result.NodeChanges = compareNodes(before.Nodes, after.Nodes)
	result.Summary.NodesChanged = len(result.NodeChanges)

	// Compare operators
	result.OperatorChanges = compareOperators(before.Operators, after.Operators)
	// Count unique operators changed (not individual field changes)
	changedOperators := map[string]struct{}{}
	for _, diff := range result.OperatorChanges {
		changedOperators[diff.Name] = struct{}{}
	}
	result.Summary.OperatorsChanged = len(changedOperators)

	// Compare namespaces
	result.NamespaceChanges = compareNamespaces(before.Namespaces, after.Namespaces)
	result.Summary.NamespacesChanged = len(result.NamespaceChanges)

	// Compare resources
	allResourceTypes := make(map[string]bool)
	for k := range before.Resources {
		allResourceTypes[k] = true
	}
	for k := range after.Resources {
		allResourceTypes[k] = true
	}

	for resourceType := range allResourceTypes {
		beforeResources := before.Resources[resourceType]
		afterResources := after.Resources[resourceType]
		diffs := compareResources(beforeResources, afterResources)
		if len(diffs) > 0 {
			result.ResourceChanges[resourceType] = diffs
			result.Summary.ResourcesChanged += len(diffs)
		}
	}

	result.Summary.TotalChanges = result.Summary.NodesChanged +
		result.Summary.OperatorsChanged +
		result.Summary.NamespacesChanged +
		result.Summary.ResourcesChanged

	return result
}

func compareNodes(before, after []NodeSnapshot) []NodeDiff {
	var diffs []NodeDiff

	beforeMap := make(map[string]NodeSnapshot)
	for _, n := range before {
		beforeMap[n.Name] = n
	}

	afterMap := make(map[string]NodeSnapshot)
	for _, n := range after {
		afterMap[n.Name] = n
	}

	// Find added and modified nodes
	for name, afterNode := range afterMap {
		if beforeNode, exists := beforeMap[name]; !exists {
			diffs = append(diffs, NodeDiff{
				Name:       name,
				ChangeType: "added",
				After:      fmt.Sprintf("Status: %s, Roles: %v, Version: %s", afterNode.Status, afterNode.Roles, afterNode.Version),
			})
		} else {
			// Check for any changes
			var changes []string
			if beforeNode.Status != afterNode.Status {
				changes = append(changes, fmt.Sprintf("Status: %s -> %s", beforeNode.Status, afterNode.Status))
			}
			if beforeNode.Version != afterNode.Version {
				changes = append(changes, fmt.Sprintf("Version: %s -> %s", beforeNode.Version, afterNode.Version))
			}
			if fmt.Sprintf("%v", beforeNode.Roles) != fmt.Sprintf("%v", afterNode.Roles) {
				changes = append(changes, fmt.Sprintf("Roles: %v -> %v", beforeNode.Roles, afterNode.Roles))
			}
			if len(changes) > 0 {
				diffs = append(diffs, NodeDiff{
					Name:       name,
					ChangeType: "modified",
					Before:     fmt.Sprintf("Status: %s, Roles: %v, Version: %s", beforeNode.Status, beforeNode.Roles, beforeNode.Version),
					After:      fmt.Sprintf("Status: %s, Roles: %v, Version: %s", afterNode.Status, afterNode.Roles, afterNode.Version),
				})
			}
		}
	}

	// Find removed nodes
	for name, beforeNode := range beforeMap {
		if _, exists := afterMap[name]; !exists {
			diffs = append(diffs, NodeDiff{
				Name:       name,
				ChangeType: "removed",
				Before:     fmt.Sprintf("Status: %s, Roles: %v", beforeNode.Status, beforeNode.Roles),
			})
		}
	}

	return diffs
}

func compareOperators(before, after []OperatorSnapshot) []OperatorDiff {
	var diffs []OperatorDiff

	beforeMap := make(map[string]OperatorSnapshot)
	for _, o := range before {
		beforeMap[o.Name] = o
	}

	afterMap := make(map[string]OperatorSnapshot)
	for _, o := range after {
		afterMap[o.Name] = o
	}

	// Find added and modified operators
	for name, afterOp := range afterMap {
		if beforeOp, exists := beforeMap[name]; !exists {
			diffs = append(diffs, OperatorDiff{
				Name:       name,
				ChangeType: "added",
				After:      formatOperatorStatus(afterOp),
			})
		} else {
			// Check for status changes
			if beforeOp.Available != afterOp.Available {
				diffs = append(diffs, OperatorDiff{
					Name:       name,
					ChangeType: "modified",
					Field:      "Available",
					Before:     fmt.Sprintf("%v", beforeOp.Available),
					After:      fmt.Sprintf("%v", afterOp.Available),
				})
			}
			if beforeOp.Degraded != afterOp.Degraded {
				diffs = append(diffs, OperatorDiff{
					Name:       name,
					ChangeType: "modified",
					Field:      "Degraded",
					Before:     fmt.Sprintf("%v", beforeOp.Degraded),
					After:      fmt.Sprintf("%v", afterOp.Degraded),
				})
			}
			if beforeOp.Progressing != afterOp.Progressing {
				diffs = append(diffs, OperatorDiff{
					Name:       name,
					ChangeType: "modified",
					Field:      "Progressing",
					Before:     fmt.Sprintf("%v", beforeOp.Progressing),
					After:      fmt.Sprintf("%v", afterOp.Progressing),
				})
			}
			if beforeOp.Version != afterOp.Version {
				diffs = append(diffs, OperatorDiff{
					Name:       name,
					ChangeType: "modified",
					Field:      "Version",
					Before:     beforeOp.Version,
					After:      afterOp.Version,
				})
			}
			// Check for condition changes
			if !slicesEqual(beforeOp.Conditions, afterOp.Conditions) {
				diffs = append(diffs, OperatorDiff{
					Name:       name,
					ChangeType: "modified",
					Field:      "Conditions",
					Before:     strings.Join(beforeOp.Conditions, ", "),
					After:      strings.Join(afterOp.Conditions, ", "),
				})
			}
		}
	}

	// Find removed operators
	for name, beforeOp := range beforeMap {
		if _, exists := afterMap[name]; !exists {
			diffs = append(diffs, OperatorDiff{
				Name:       name,
				ChangeType: "removed",
				Before:     formatOperatorStatus(beforeOp),
			})
		}
	}

	return diffs
}

func formatOperatorStatus(op OperatorSnapshot) string {
	return fmt.Sprintf("Available=%v, Degraded=%v, Progressing=%v, Version=%s",
		op.Available, op.Degraded, op.Progressing, op.Version)
}

func compareNamespaces(before, after []NamespaceSnapshot) []NamespaceDiff {
	var diffs []NamespaceDiff

	beforeMap := make(map[string]NamespaceSnapshot)
	for _, n := range before {
		beforeMap[n.Name] = n
	}

	afterMap := make(map[string]NamespaceSnapshot)
	for _, n := range after {
		afterMap[n.Name] = n
	}

	// Find added and modified namespaces
	for name, afterNs := range afterMap {
		if beforeNs, exists := beforeMap[name]; !exists {
			diffs = append(diffs, NamespaceDiff{
				Name:       name,
				ChangeType: "added",
				After:      fmt.Sprintf("Status: %s", afterNs.Status),
			})
		} else {
			// Check for status or label changes
			statusChanged := beforeNs.Status != afterNs.Status
			labelsChanged := fmt.Sprintf("%v", beforeNs.Labels) != fmt.Sprintf("%v", afterNs.Labels)
			if statusChanged || labelsChanged {
				diffs = append(diffs, NamespaceDiff{
					Name:       name,
					ChangeType: "modified",
					Before:     fmt.Sprintf("Status: %s, Labels: %v", beforeNs.Status, beforeNs.Labels),
					After:      fmt.Sprintf("Status: %s, Labels: %v", afterNs.Status, afterNs.Labels),
				})
			}
		}
	}

	// Find removed namespaces
	for name, beforeNs := range beforeMap {
		if _, exists := afterMap[name]; !exists {
			diffs = append(diffs, NamespaceDiff{
				Name:       name,
				ChangeType: "removed",
				Before:     fmt.Sprintf("Status: %s", beforeNs.Status),
			})
		}
	}

	return diffs
}

func compareResources(before, after []ResourceInfo) []ResourceDiff {
	var diffs []ResourceDiff

	beforeMap := make(map[string]ResourceInfo)
	for _, r := range before {
		key := fmt.Sprintf("%s/%s/%s", r.Namespace, r.Kind, r.Name)
		beforeMap[key] = r
	}

	afterMap := make(map[string]ResourceInfo)
	for _, r := range after {
		key := fmt.Sprintf("%s/%s/%s", r.Namespace, r.Kind, r.Name)
		afterMap[key] = r
	}

	// Find added and modified resources
	for key, afterRes := range afterMap {
		if beforeRes, exists := beforeMap[key]; !exists {
			diffs = append(diffs, ResourceDiff{
				Name:       afterRes.Name,
				Namespace:  afterRes.Namespace,
				ChangeType: "added",
				After:      fmt.Sprintf("Status: %s", afterRes.Status),
			})
		} else if beforeRes.Status != afterRes.Status {
			diffs = append(diffs, ResourceDiff{
				Name:       afterRes.Name,
				Namespace:  afterRes.Namespace,
				ChangeType: "modified",
				Before:     fmt.Sprintf("Status: %s", beforeRes.Status),
				After:      fmt.Sprintf("Status: %s", afterRes.Status),
			})
		}
	}

	// Find removed resources
	for key, beforeRes := range beforeMap {
		if _, exists := afterMap[key]; !exists {
			diffs = append(diffs, ResourceDiff{
				Name:       beforeRes.Name,
				Namespace:  beforeRes.Namespace,
				ChangeType: "removed",
				Before:     fmt.Sprintf("Status: %s", beforeRes.Status),
			})
		}
	}

	return diffs
}

func (o *diffOptions) printDiff(result *DiffResult) error {
	if o.OutputJSON {
		output, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(output))
		return nil
	}

	// Print human-readable diff
	fmt.Printf("\n╔══════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                    CLUSTER SNAPSHOT DIFF                      ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Before: %-54s ║\n", result.BeforeSnapshot)
	fmt.Printf("║ After:  %-54s ║\n", result.AfterSnapshot)
	fmt.Printf("╚══════════════════════════════════════════════════════════════╝\n\n")

	fmt.Printf("SUMMARY\n")
	fmt.Printf("───────\n")
	fmt.Printf("Total Changes:     %d\n", result.Summary.TotalChanges)
	fmt.Printf("Nodes Changed:     %d\n", result.Summary.NodesChanged)
	fmt.Printf("Operators Changed: %d\n", result.Summary.OperatorsChanged)
	fmt.Printf("Namespaces Changed: %d\n", result.Summary.NamespacesChanged)
	fmt.Printf("Resources Changed: %d\n\n", result.Summary.ResourcesChanged)

	if result.Summary.TotalChanges == 0 {
		fmt.Println("✓ No changes detected between snapshots.")
		return nil
	}

	// Print node changes
	if len(result.NodeChanges) > 0 {
		fmt.Println("NODE CHANGES")
		fmt.Println("────────────")
		for _, d := range result.NodeChanges {
			printChange(d.Name, d.ChangeType, d.Before, d.After)
		}
		fmt.Println()
	}

	// Print operator changes
	if len(result.OperatorChanges) > 0 {
		fmt.Println("OPERATOR CHANGES")
		fmt.Println("────────────────")
		for _, d := range result.OperatorChanges {
			name := d.Name
			if d.Field != "" {
				name = fmt.Sprintf("%s [%s]", d.Name, d.Field)
			}
			printChange(name, d.ChangeType, d.Before, d.After)
		}
		fmt.Println()
	}

	// Print namespace changes
	if len(result.NamespaceChanges) > 0 {
		fmt.Println("NAMESPACE CHANGES")
		fmt.Println("─────────────────")
		for _, d := range result.NamespaceChanges {
			printChange(d.Name, d.ChangeType, d.Before, d.After)
		}
		fmt.Println()
	}

	// Print resource changes
	for resourceType, changes := range result.ResourceChanges {
		if len(changes) > 0 {
			fmt.Printf("%s CHANGES\n", strings.ToUpper(resourceType))
			fmt.Println(strings.Repeat("─", len(resourceType)+8))
			for _, d := range changes {
				name := d.Name
				if d.Namespace != "" {
					name = fmt.Sprintf("%s/%s", d.Namespace, d.Name)
				}
				printChange(name, d.ChangeType, d.Before, d.After)
			}
			fmt.Println()
		}
	}

	return nil
}

func printChange(name, changeType, before, after string) {
	var symbol string
	switch changeType {
	case "added":
		symbol = "+"
	case "removed":
		symbol = "-"
	case "modified":
		symbol = "~"
	}

	fmt.Printf("  %s %s\n", symbol, name)
	if before != "" {
		fmt.Printf("      Before: %s\n", before)
	}
	if after != "" {
		fmt.Printf("      After:  %s\n", after)
	}
}

// slicesEqual compares two string slices for equality
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// warnCaptureErrors prints warnings about capture errors that could cause false diffs
func warnCaptureErrors(snapshot *ClusterSnapshot, label, filename string) {
	if len(snapshot.Metadata.CaptureErrors) > 0 {
		fmt.Fprintf(os.Stderr, "[WARN] %s snapshot (%s) has capture errors:\n", label, filename)
		for section, errMsg := range snapshot.Metadata.CaptureErrors {
			fmt.Fprintf(os.Stderr, "       - %s: %s\n", section, errMsg)
		}
		fmt.Fprintln(os.Stderr, "[WARN] Diff results for failed sections may show false additions/removals")
	}
}
