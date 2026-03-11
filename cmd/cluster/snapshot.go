package cluster

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// ClusterSnapshot represents a point-in-time capture of cluster state
type ClusterSnapshot struct {
	Metadata   SnapshotMetadata          `yaml:"metadata"`
	Namespaces []NamespaceSnapshot       `yaml:"namespaces,omitempty"`
	Nodes      []NodeSnapshot            `yaml:"nodes,omitempty"`
	Operators  []OperatorSnapshot        `yaml:"operators,omitempty"`
	Resources  map[string][]ResourceInfo `yaml:"resources,omitempty"`
}

// SnapshotMetadata contains information about when/how the snapshot was taken
type SnapshotMetadata struct {
	ClusterID     string            `yaml:"clusterId"`
	ClusterName   string            `yaml:"clusterName"`
	Timestamp     time.Time         `yaml:"timestamp"`
	Version       string            `yaml:"version"`
	Platform      string            `yaml:"platform"`
	IsHCP         bool              `yaml:"isHCP"`
	CaptureErrors map[string]string `yaml:"captureErrors,omitempty"`
}

// NamespaceSnapshot captures namespace state
type NamespaceSnapshot struct {
	Name   string            `yaml:"name"`
	Status string            `yaml:"status"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

// NodeSnapshot captures node state
type NodeSnapshot struct {
	Name       string            `yaml:"name"`
	Status     string            `yaml:"status"`
	Roles      []string          `yaml:"roles,omitempty"`
	Version    string            `yaml:"version"`
	Conditions []string          `yaml:"conditions,omitempty"`
	Labels     map[string]string `yaml:"labels,omitempty"`
}

// OperatorSnapshot captures ClusterOperator state
type OperatorSnapshot struct {
	Name        string   `yaml:"name"`
	Available   bool     `yaml:"available"`
	Progressing bool     `yaml:"progressing"`
	Degraded    bool     `yaml:"degraded"`
	Version     string   `yaml:"version,omitempty"`
	Conditions  []string `yaml:"conditions,omitempty"`
}

// ResourceInfo captures basic resource information
type ResourceInfo struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace,omitempty"`
	Kind      string `yaml:"kind"`
	Status    string `yaml:"status,omitempty"`
}

// snapshotOptions holds the options for the snapshot command
type snapshotOptions struct {
	ClusterID      string
	OutputFile     string
	IncludeSecrets bool
	Namespaces     []string
	ResourceTypes  []string
}

func newCmdSnapshot() *cobra.Command {
	opts := &snapshotOptions{}

	snapshotCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Capture a point-in-time snapshot of cluster state",
		Long: `Capture a point-in-time snapshot of cluster state for evidence collection.

This command captures the current state of key cluster resources including:
- Namespace states
- Node conditions and readiness  
- ClusterOperator status
- Custom resources (optional)

The snapshot can be saved to a YAML file and later compared using 
'osdctl cluster diff' to identify changes during feature testing.`,
		Example: `  # Capture cluster snapshot to a file
  osdctl cluster snapshot -C <cluster-id> -o before.yaml

  # Capture snapshot with specific namespaces
  osdctl cluster snapshot -C <cluster-id> -o snapshot.yaml --namespaces openshift-monitoring,openshift-operators

  # Capture additional resource types
  osdctl cluster snapshot -C <cluster-id> -o snapshot.yaml --resources pods,deployments,services`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.run()
		},
	}

	snapshotCmd.Flags().StringVarP(&opts.ClusterID, "cluster-id", "C", "", "Cluster ID (internal, external, or name)")
	snapshotCmd.Flags().StringVarP(&opts.OutputFile, "output", "o", "", "Output file path (YAML format)")
	snapshotCmd.Flags().StringSliceVar(&opts.Namespaces, "namespaces", []string{}, "Specific namespaces to include (default: all openshift-* namespaces)")
	snapshotCmd.Flags().StringSliceVar(&opts.ResourceTypes, "resources", []string{}, "Additional resource types to capture (e.g., pods,deployments)")
	cmdutil.CheckErr(snapshotCmd.MarkFlagRequired("cluster-id"))
	cmdutil.CheckErr(snapshotCmd.MarkFlagRequired("output"))

	return snapshotCmd
}

func (o *snapshotOptions) run() error {
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

	isHCP := cluster.Hypershift().Enabled()
	clusterType := "Classic"
	if isHCP {
		clusterType = "HCP (Hosted Control Plane)"
	}
	fmt.Printf("[INFO] Creating snapshot for cluster: %s (%s) - %s\n", cluster.Name(), cluster.ID(), clusterType)

	captureErrors := make(map[string]string)

	snapshot := &ClusterSnapshot{
		Metadata: SnapshotMetadata{
			ClusterID:   cluster.ID(),
			ClusterName: cluster.Name(),
			Timestamp:   time.Now().UTC(),
			Version:     cluster.OpenshiftVersion(),
			Platform:    cluster.CloudProvider().ID(),
			IsHCP:       isHCP,
		},
		Resources: make(map[string][]ResourceInfo),
	}

	if isHCP {
		fmt.Println("[INFO] HCP cluster detected - note that only worker nodes will be visible")
	}

	// Capture nodes
	fmt.Println("[INFO] Capturing node states...")
	nodes, err := o.captureNodes()
	if err != nil {
		fmt.Printf("[WARN] Failed to capture nodes: %v\n", err)
		captureErrors["nodes"] = err.Error()
	} else {
		snapshot.Nodes = nodes
	}

	// Capture namespaces
	fmt.Println("[INFO] Capturing namespace states...")
	namespaces, err := o.captureNamespaces()
	if err != nil {
		fmt.Printf("[WARN] Failed to capture namespaces: %v\n", err)
		captureErrors["namespaces"] = err.Error()
	} else {
		snapshot.Namespaces = namespaces
	}

	// Capture cluster operators
	fmt.Println("[INFO] Capturing ClusterOperator states...")
	operators, err := o.captureClusterOperators()
	if err != nil {
		fmt.Printf("[WARN] Failed to capture cluster operators: %v\n", err)
		captureErrors["operators"] = err.Error()
	} else {
		snapshot.Operators = operators
	}

	// Capture additional resources if specified
	for _, resourceType := range o.ResourceTypes {
		fmt.Printf("[INFO] Capturing %s...\n", resourceType)
		resources, err := o.captureResources(resourceType)
		if err != nil {
			fmt.Printf("[WARN] Failed to capture %s: %v\n", resourceType, err)
			captureErrors[resourceType] = err.Error()
			continue
		}
		snapshot.Resources[resourceType] = resources
	}

	// Store capture errors in metadata
	if len(captureErrors) > 0 {
		snapshot.Metadata.CaptureErrors = captureErrors
	}

	// Fail if all core sections failed
	if len(snapshot.Nodes) == 0 && len(snapshot.Namespaces) == 0 && len(snapshot.Operators) == 0 {
		if len(captureErrors) > 0 {
			return fmt.Errorf("failed to capture any cluster state: %v", captureErrors)
		}
	}

	// Write snapshot to file
	if err := o.writeSnapshot(snapshot); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}

	fmt.Printf("[INFO] Snapshot saved to: %s\n", o.OutputFile)
	return nil
}

func (o *snapshotOptions) captureNodes() ([]NodeSnapshot, error) {
	output, err := exec.CommandContext(context.TODO(), "oc", "get", "nodes", "-o", "json").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("oc get nodes failed: %w: %s", err, strings.TrimSpace(string(output)))
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
				NodeInfo struct {
					KubeletVersion string `json:"kubeletVersion"`
				} `json:"nodeInfo"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := yaml.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var nodes []NodeSnapshot
	for _, item := range result.Items {
		node := NodeSnapshot{
			Name:    item.Metadata.Name,
			Version: item.Status.NodeInfo.KubeletVersion,
			Labels:  item.Metadata.Labels,
		}

		// Extract roles from labels
		for label := range item.Metadata.Labels {
			if strings.HasPrefix(label, "node-role.kubernetes.io/") {
				role := strings.TrimPrefix(label, "node-role.kubernetes.io/")
				node.Roles = append(node.Roles, role)
			}
		}

		// Check node conditions
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

func (o *snapshotOptions) captureNamespaces() ([]NamespaceSnapshot, error) {
	args := []string{"get", "namespaces", "-o", "json"}

	output, err := exec.CommandContext(context.TODO(), "oc", args...).CombinedOutput() //#nosec G204 -- args are constructed from trusted input
	if err != nil {
		return nil, fmt.Errorf("oc get namespaces failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	var result struct {
		Items []struct {
			Metadata struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
			Status struct {
				Phase string `json:"phase"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := yaml.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var namespaces []NamespaceSnapshot
	for _, item := range result.Items {
		// Filter to openshift-* namespaces if no specific namespaces provided
		if len(o.Namespaces) == 0 {
			if !strings.HasPrefix(item.Metadata.Name, "openshift-") {
				continue
			}
		} else {
			found := false
			for _, ns := range o.Namespaces {
				if item.Metadata.Name == ns {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		namespaces = append(namespaces, NamespaceSnapshot{
			Name:   item.Metadata.Name,
			Status: item.Status.Phase,
			Labels: item.Metadata.Labels,
		})
	}

	return namespaces, nil
}

func (o *snapshotOptions) captureClusterOperators() ([]OperatorSnapshot, error) {
	output, err := exec.CommandContext(context.TODO(), "oc", "get", "clusteroperators", "-o", "json").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("oc get clusteroperators failed: %w: %s", err, strings.TrimSpace(string(output)))
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

	if err := yaml.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var operators []OperatorSnapshot
	for _, item := range result.Items {
		operator := OperatorSnapshot{
			Name: item.Metadata.Name,
		}

		for _, cond := range item.Status.Conditions {
			operator.Conditions = append(operator.Conditions, fmt.Sprintf("%s=%s", cond.Type, cond.Status))
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

func (o *snapshotOptions) captureResources(resourceType string) ([]ResourceInfo, error) {
	output, err := exec.CommandContext(context.TODO(), "oc", "get", resourceType, "--all-namespaces", "-o", "json").CombinedOutput() //#nosec G204 -- resourceType is user-provided but filtered
	if err != nil {
		return nil, fmt.Errorf("oc get %s failed: %w: %s", resourceType, err, strings.TrimSpace(string(output)))
	}

	var result struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Status struct {
				Phase string `json:"phase"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := yaml.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var resources []ResourceInfo
	for _, item := range result.Items {
		resources = append(resources, ResourceInfo{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
			Kind:      resourceType,
			Status:    item.Status.Phase,
		})
	}

	return resources, nil
}

func (o *snapshotOptions) writeSnapshot(snapshot *ClusterSnapshot) error {
	// Ensure directory exists
	dir := filepath.Dir(o.OutputFile)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	data, err := yaml.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	if err := os.WriteFile(o.OutputFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
