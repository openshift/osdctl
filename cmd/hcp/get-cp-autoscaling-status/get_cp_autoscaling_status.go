package getcpautoscalingstatus

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"time"

	sdk "github.com/openshift-online/ocm-sdk-go"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	annotationResourceBasedAutoscaling = "hypershift.openshift.io/resource-based-cp-auto-scaling"
	annotationClusterSizeOverride      = "hypershift.openshift.io/cluster-size-override"
	annotationRecommendedClusterSize   = "hypershift.openshift.io/recommended-cluster-size"

	labelHostedClusterSize = "hypershift.openshift.io/hosted-cluster-size"
	labelClusterID         = "api.openshift.com/id"
)

type options struct {
	mgmtClusterID string
	output        string
	showOnly      string
	noHeaders     bool
}

type clusterInfo struct {
	ClusterID             string `json:"cluster_id" yaml:"cluster_id"`
	ClusterName           string `json:"cluster_name" yaml:"cluster_name"`
	Namespace             string `json:"namespace" yaml:"namespace"`
	AutoscalingEnabled    bool   `json:"autoscaling_enabled" yaml:"autoscaling_enabled"`
	HasOverrideAnnotation bool   `json:"has_override" yaml:"has_override"`
	CurrentSize           string `json:"current_size" yaml:"current_size"`
	RecommendedSize       string `json:"recommended_size" yaml:"recommended_size"`
}

type auditResults struct {
	Timestamp         time.Time     `json:"timestamp" yaml:"timestamp"`
	ManagementCluster string        `json:"management_cluster" yaml:"management_cluster"`
	TotalClusters     int           `json:"total_clusters" yaml:"total_clusters"`
	Clusters          []clusterInfo `json:"clusters" yaml:"clusters"`
}

func newCmdAutoscalingAudit() *cobra.Command {
	opts := &options{}

	cmd := &cobra.Command{
		Use:   "get-cp-autoscaling-status",
		Short: "Get control plane autoscaling status for hosted clusters on a management cluster",
		Long: `Query a single HCP management cluster to retrieve autoscaling status for all hosted clusters.

This command is useful for checking the autoscaling configuration status of hosted clusters
on a specific management cluster during day-to-day operations.`,
		Example: `
  # Get autoscaling status for all hosted clusters on a management cluster
  osdctl hcp get-cp-autoscaling-status --mgmt-cluster-id ${MGMT_CLUSTER_ID}

  # Get status with CSV output
  osdctl hcp get-cp-autoscaling-status --mgmt-cluster-id ${MGMT_CLUSTER_ID} --output csv > status.csv

  # Show only clusters ready for migration
  osdctl hcp get-cp-autoscaling-status --mgmt-cluster-id ${MGMT_CLUSTER_ID} --show-only ready-for-migration

  # Show only clusters that need annotation removal
  osdctl hcp get-cp-autoscaling-status --mgmt-cluster-id ${MGMT_CLUSTER_ID} --show-only needs-removal

  # Show only clusters safe to remove override
  osdctl hcp get-cp-autoscaling-status --mgmt-cluster-id ${MGMT_CLUSTER_ID} --show-only safe-to-remove-override`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.run(context.Background())
		},
	}

	cmd.Flags().StringVar(&opts.mgmtClusterID, "mgmt-cluster-id", "",
		"Management cluster ID or name (required)")
	cmd.Flags().StringVar(&opts.output, "output", "text",
		"Output format: text, json, yaml, csv")
	cmd.Flags().StringVar(&opts.showOnly, "show-only", "",
		"Filter output: needs-removal, ready-for-migration, safe-to-remove-override")
	cmd.Flags().BoolVar(&opts.noHeaders, "no-headers", false,
		"Skip table headers in output")

	if err := cmd.MarkFlagRequired("mgmt-cluster-id"); err != nil {
		panic(fmt.Sprintf("failed to mark flag as required: %v", err))
	}

	return cmd
}

func (o *options) run(ctx context.Context) error {
	validOutputs := map[string]bool{"text": true, "json": true, "yaml": true, "csv": true}
	if !validOutputs[o.output] {
		return fmt.Errorf("invalid output format '%s'. Valid options: text, json, yaml, csv", o.output)
	}

	if o.showOnly != "" {
		validFilters := map[string]bool{"needs-removal": true, "ready-for-migration": true, "safe-to-remove-override": true}
		if !validFilters[o.showOnly] {
			return fmt.Errorf("invalid show-only filter '%s'. Valid options: needs-removal, ready-for-migration, safe-to-remove-override", o.showOnly)
		}
	}

	connection, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("failed to create OCM connection: %v", err)
	}
	defer connection.Close()

	return o.auditManagementCluster(ctx, connection)
}

func (o *options) auditManagementCluster(ctx context.Context, conn *sdk.Connection) error {
	if err := utils.IsValidClusterKey(o.mgmtClusterID); err != nil {
		return err
	}

	cluster, err := utils.GetCluster(conn, o.mgmtClusterID)
	if err != nil {
		return fmt.Errorf("failed to get cluster: %v", err)
	}

	isMC, err := utils.IsManagementCluster(cluster.ID())
	if err != nil {
		return fmt.Errorf("failed to verify management cluster: %v", err)
	}
	if !isMC {
		return fmt.Errorf("cluster %s is not a management cluster", cluster.ID())
	}

	resolvedMgmtClusterID := cluster.ID()
	resolvedMgmtClusterName := cluster.Name()

	scheme := runtime.NewScheme()
	if err := hypershiftv1beta1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add hypershift scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add core v1 scheme: %v", err)
	}

	mgmtClient, err := k8s.NewWithConn(resolvedMgmtClusterID, client.Options{Scheme: scheme}, conn)
	if err != nil {
		return fmt.Errorf("failed to create management cluster client: %v", err)
	}

	var namespaces []corev1.Namespace
	maxRetries := 3
	retryDelay := 2 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		namespaces, err = listOcmNamespaces(ctx, mgmtClient)
		if err == nil {
			break
		}

		if attempt == maxRetries {
			return fmt.Errorf("failed to list namespaces after %d attempts (cluster may be unreachable): %v", maxRetries, err)
		}

		time.Sleep(retryDelay)
	}

	results := &auditResults{
		Timestamp:         time.Now(),
		ManagementCluster: resolvedMgmtClusterName,
		Clusters:          []clusterInfo{},
	}

	for _, ns := range namespaces {
		info, err := auditNamespace(ctx, mgmtClient, ns.Name)
		if err != nil {
			fmt.Printf("Warning: failed to audit namespace %s: %v\n", ns.Name, err)
			continue
		}

		results.Clusters = append(results.Clusters, *info)
	}

	results.TotalClusters = len(results.Clusters)

	if o.showOnly != "" {
		results = o.applyFilter(results)
	}

	return o.outputResults(results)
}

func listOcmNamespaces(ctx context.Context, kubeClient client.Client) ([]corev1.Namespace, error) {
	nsList := &corev1.NamespaceList{}
	if err := kubeClient.List(ctx, nsList); err != nil {
		return nil, err
	}

	var filtered []corev1.Namespace
	ocmNamespacePattern := regexp.MustCompile(`^ocm-(production|staging)-[a-zA-Z0-9]+$`)

	for _, ns := range nsList.Items {
		if ocmNamespacePattern.MatchString(ns.Name) {
			filtered = append(filtered, ns)
		}
	}

	return filtered, nil
}

func auditNamespace(ctx context.Context, kubeClient client.Client, namespace string) (*clusterInfo, error) {
	hc, err := getHostedClusterInNamespace(ctx, kubeClient, namespace)
	if err != nil {
		return nil, err
	}

	clusterID := hc.Labels[labelClusterID]
	currentSize := hc.Labels[labelHostedClusterSize]
	if currentSize == "" {
		currentSize = "N/A"
	}

	autoScaling, hasAutoScaling := hc.Annotations[annotationResourceBasedAutoscaling]
	autoscalingEnabled := hasAutoScaling && autoScaling == "true"

	_, hasOverride := hc.Annotations[annotationClusterSizeOverride]

	recommendedSize := hc.Annotations[annotationRecommendedClusterSize]
	if recommendedSize == "" {
		recommendedSize = "N/A"
	}

	return &clusterInfo{
		ClusterID:             clusterID,
		ClusterName:           hc.Name,
		Namespace:             namespace,
		AutoscalingEnabled:    autoscalingEnabled,
		HasOverrideAnnotation: hasOverride,
		CurrentSize:           currentSize,
		RecommendedSize:       recommendedSize,
	}, nil
}

func getHostedClusterInNamespace(ctx context.Context, kubeClient client.Client, namespace string) (*hypershiftv1beta1.HostedCluster, error) {
	hcList := &hypershiftv1beta1.HostedClusterList{}
	listOpts := []client.ListOption{client.InNamespace(namespace)}

	if err := kubeClient.List(ctx, hcList, listOpts...); err != nil {
		return nil, err
	}

	if len(hcList.Items) == 0 {
		return nil, fmt.Errorf("no HostedCluster found")
	}

	if len(hcList.Items) > 1 {
		return nil, fmt.Errorf("found %d HostedClusters, expected 1", len(hcList.Items))
	}

	return &hcList.Items[0], nil
}

func (o *options) applyFilter(results *auditResults) *auditResults {
	filtered := &auditResults{
		Timestamp:         results.Timestamp,
		ManagementCluster: results.ManagementCluster,
		Clusters:          []clusterInfo{},
	}

	for _, cluster := range results.Clusters {
		isSafeToRemoveOverride := cluster.AutoscalingEnabled &&
			cluster.HasOverrideAnnotation &&
			cluster.RecommendedSize != "" &&
			cluster.RecommendedSize != "N/A" &&
			cluster.CurrentSize == cluster.RecommendedSize

		switch o.showOnly {
		case "needs-removal":
			if cluster.HasOverrideAnnotation {
				filtered.Clusters = append(filtered.Clusters, cluster)
			}
		case "ready-for-migration":
			if !cluster.AutoscalingEnabled {
				filtered.Clusters = append(filtered.Clusters, cluster)
			}
		case "safe-to-remove-override":
			if isSafeToRemoveOverride {
				filtered.Clusters = append(filtered.Clusters, cluster)
			}
		}
	}

	filtered.TotalClusters = len(filtered.Clusters)
	return filtered
}

func (o *options) outputResults(results *auditResults) error {
	switch o.output {
	case "text":
		return o.printTable(results)
	case "json":
		return o.printJSON(results)
	case "yaml":
		return o.printYAML(results)
	case "csv":
		return o.printCSV(results)
	default:
		return fmt.Errorf("unsupported output format: %s", o.output)
	}
}

func (o *options) printTable(results *auditResults) error {
	fmt.Printf("\n=== Management Cluster: %s ===\n", results.ManagementCluster)
	fmt.Printf("Timestamp: %s\n", results.Timestamp.Format(time.RFC3339))
	fmt.Printf("Total Hosted Clusters: %d\n\n", results.TotalClusters)

	if len(results.Clusters) == 0 {
		fmt.Println("No hosted clusters found")
		return nil
	}

	sort.Slice(results.Clusters, func(i, j int) bool {
		return results.Clusters[i].ClusterName < results.Clusters[j].ClusterName
	})

	p := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')

	if !o.noHeaders {
		p.AddRow([]string{
			"CLUSTER ID",
			"CLUSTER NAME",
			"NAMESPACE",
			"AUTOSCALING",
			"HAS OVERRIDE",
			"CURRENT SIZE",
			"RECOMMENDED SIZE",
		})
	}

	for _, c := range results.Clusters {
		autoscalingStr := "❌"
		if c.AutoscalingEnabled {
			autoscalingStr = "✅"
		}

		overrideStr := "❌"
		if c.HasOverrideAnnotation {
			overrideStr = "✅"
		}

		p.AddRow([]string{
			c.ClusterID,
			c.ClusterName,
			c.Namespace,
			autoscalingStr,
			overrideStr,
			c.CurrentSize,
			c.RecommendedSize,
		})
	}

	p.Flush()
	fmt.Println()

	return nil
}

func (o *options) printJSON(results *auditResults) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(results)
}

func (o *options) printYAML(results *auditResults) error {
	data, err := yaml.Marshal(results)
	if err != nil {
		return fmt.Errorf("failed to marshal to YAML: %v", err)
	}
	fmt.Print(string(data))
	return nil
}

func (o *options) printCSV(results *auditResults) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	if !o.noHeaders {
		if err := w.Write([]string{
			"cluster_id",
			"cluster_name",
			"namespace",
			"autoscaling_enabled",
			"has_override",
			"current_size",
			"recommended_size",
		}); err != nil {
			return fmt.Errorf("failed to write CSV header: %v", err)
		}
	}

	for _, c := range results.Clusters {
		autoscalingStr := "false"
		if c.AutoscalingEnabled {
			autoscalingStr = "true"
		}

		overrideStr := "false"
		if c.HasOverrideAnnotation {
			overrideStr = "true"
		}

		if err := w.Write([]string{
			c.ClusterID,
			c.ClusterName,
			c.Namespace,
			autoscalingStr,
			overrideStr,
			c.CurrentSize,
			c.RecommendedSize,
		}); err != nil {
			return fmt.Errorf("failed to write CSV row: %v", err)
		}
	}

	return nil
}
