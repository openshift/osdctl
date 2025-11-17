package resize

import (
	"context"
	"errors"
	"fmt"
	"time"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	resizeRequestServingServiceLogTemplate = "https://raw.githubusercontent.com/openshift/managed-notifications/master/hcp/RequestServingNode_resized.json"
)

type requestServingNodesOpts struct {
	clusterID string
	cluster   *cmv1.Cluster
	size      string
	reason    string

	// mgmtClient is a K8s client to management cluster
	mgmtClient client.Client

	// mgmtClientAdmin is a K8s client to management cluster with elevation
	mgmtClientAdmin client.Client
}

type clusterSize struct {
	Name     string       `json:"name"`
	Criteria sizeCriteria `json:"criteria"`
}

type sizeCriteria struct {
	From int `json:"from"`
	To   int `json:"to,omitempty"`
}

func newCmdResizeRequestServingNodes() *cobra.Command {
	opts := &requestServingNodesOpts{}
	cmd := &cobra.Command{
		Use:   "request-serving-nodes",
		Short: "Resize a ROSA HCP cluster's request-serving nodes",
		Long:  `Resize a ROSA HCP cluster's request-serving nodes by applying a cluster-size-override annotation`,
		Example: `
  # Resize a ROSA HCP cluster's request-serving nodes to the next size
  osdctl cluster resize request-serving-nodes --cluster-id "${CLUSTER_ID}" --reason "${OHSS}"

  # Resize a ROSA HCP cluster's request-serving nodes to a specific size
  osdctl cluster resize request-serving-nodes --cluster-id "${CLUSTER_ID}" --size m54xl --reason "${OHSS}"
`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.run(context.Background())
		},
	}

	cmd.Flags().StringVarP(&opts.clusterID, "cluster-id", "C", "", "The internal ID of the cluster to perform actions on")
	cmd.Flags().StringVar(&opts.size, "size", "", "The target request-serving node size (e.g. m54xl). If not specified, will auto-select the next size up")
	cmd.Flags().StringVar(&opts.reason, "reason", "", "The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)")
	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}

func (r *requestServingNodesOpts) run(ctx context.Context) error {
	// Validate cluster key
	if err := utils.IsValidClusterKey(r.clusterID); err != nil {
		return err
	}

	// Create OCM connection and get cluster
	connection, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer connection.Close()

	cluster, err := utils.GetCluster(connection, r.clusterID)
	if err != nil {
		return err
	}
	r.cluster = cluster
	r.clusterID = cluster.ID()

	// Confirm the cluster is an HCP cluster
	if !cluster.Hypershift().Enabled() {
		return errors.New("this command is only for HCP (Hosted Control Plane) clusters")
	}

	printer.PrintlnGreen(fmt.Sprintf("Cluster %s is an HCP cluster", cluster.Name()))

	// Get management cluster
	mgmtCluster, err := utils.GetManagementCluster(r.clusterID)
	if err != nil {
		return fmt.Errorf("failed to get management cluster: %v", err)
	}

	printer.PrintlnGreen(fmt.Sprintf("Management cluster: %s", mgmtCluster.Name()))

	// Get management cluster ID for client connection
	mgmtClusterID := mgmtCluster.ID()

	// Create Kubernetes client scheme with HyperShift API
	scheme := runtime.NewScheme()
	if err := hypershiftv1beta1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add hypershift scheme: %v", err)
	}

	// Create client to management cluster using backplane SDK
	printer.PrintlnGreen("Creating management cluster client...")
	mgmtClient, err := k8s.New(mgmtClusterID, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create management cluster client: %v", err)
	}
	r.mgmtClient = mgmtClient

	// Create admin client with elevation for management cluster
	mgmtClientAdmin, err := k8s.NewAsBackplaneClusterAdmin(mgmtClusterID, client.Options{Scheme: scheme}, r.reason)
	if err != nil {
		return fmt.Errorf("failed to create admin management cluster client: %v", err)
	}
	r.mgmtClientAdmin = mgmtClientAdmin

	// Get HCP namespace (for node monitoring)
	hcpNamespace, err := utils.GetHCPNamespace(r.clusterID)
	if err != nil {
		return fmt.Errorf("failed to get HCP namespace: %v", err)
	}

	printer.PrintlnGreen(fmt.Sprintf("HCP namespace: %s", hcpNamespace))

	// Find the HostedCluster object by searching with label across all namespaces
	hostedCluster, err := r.findHostedCluster(ctx, r.clusterID)
	if err != nil {
		return fmt.Errorf("failed to find hostedcluster: %v", err)
	}

	hcNamespace := hostedCluster.Namespace
	hcName := hostedCluster.Name

	printer.PrintlnGreen(fmt.Sprintf("HostedCluster namespace: %s", hcNamespace))
	printer.PrintlnGreen(fmt.Sprintf("HostedCluster name: %s", hcName))

	// Get current hosted-cluster-size from label
	currentSize := hostedCluster.Labels["hypershift.openshift.io/hosted-cluster-size"]
	if currentSize == "" {
		return errors.New("hosted-cluster-size label not found on HostedCluster")
	}

	printer.PrintlnGreen(fmt.Sprintf("Current hosted-cluster-size: %s", currentSize))

	// Fetch valid sizes from clustersizingconfigurations
	availableSizes, err := r.getAvailableSizes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get available sizes: %v", err)
	}

	// Display available sizes
	fmt.Println("\nAvailable sizes:")
	for _, size := range availableSizes {
		toStr := "∞"
		if size.Criteria.To > 0 {
			toStr = fmt.Sprintf("%d", size.Criteria.To)
		}
		marker := ""
		if size.Name == currentSize {
			marker = " (current)"
		}
		fmt.Printf("  %s -> worker pool size: %d to %s%s\n", size.Name, size.Criteria.From, toStr, marker)
	}

	// Determine target size
	targetSize := r.size
	if targetSize == "" {
		// Auto-select next size up
		targetSize, err = r.getNextSize(currentSize, availableSizes)
		if err != nil {
			return err
		}
		printer.PrintlnGreen(fmt.Sprintf("\nAuto-selected next size: %s", targetSize))
	} else {
		// Validate user-provided size
		if !r.isValidSize(targetSize, availableSizes) {
			fmt.Printf("\nError: Invalid size '%s'\n\n", targetSize)
			fmt.Println("Valid size options:")
			for _, size := range availableSizes {
				toStr := "∞"
				if size.Criteria.To > 0 {
					toStr = fmt.Sprintf("%d", size.Criteria.To)
				}
				fmt.Printf("  - %s (for %d to %s worker nodes)\n", size.Name, size.Criteria.From, toStr)
			}
			return fmt.Errorf("size '%s' is not a valid option", targetSize)
		}
		printer.PrintlnGreen(fmt.Sprintf("\nUsing specified size: %s", targetSize))
	}

	if targetSize == currentSize {
		return fmt.Errorf("target size '%s' is the same as current size '%s'. No resize needed", targetSize, currentSize)
	}

	// Prompt user to confirm
	fmt.Printf("\nThis will resize cluster %s from %s to %s\n", cluster.Name(), currentSize, targetSize)
	if !utils.ConfirmPrompt() {
		return errors.New("resize cancelled by user")
	}

	// Apply the cluster-size-override annotation
	printer.PrintlnGreen("\nApplying cluster-size-override annotation...")
	if err := r.applyClusterSizeOverride(ctx, hostedCluster, targetSize); err != nil {
		return fmt.Errorf("failed to apply cluster-size-override annotation: %v", err)
	}

	printer.PrintlnGreen("Annotation applied successfully. Waiting for new nodes to be provisioned...")

	// Wait a bit for the operator to start processing
	time.Sleep(10 * time.Second)

	// Verify the annotation was applied by re-fetching the HostedCluster
	updatedHC := &hypershiftv1beta1.HostedCluster{}
	if err := r.mgmtClient.Get(ctx, client.ObjectKey{Namespace: hcNamespace, Name: hcName}, updatedHC); err != nil {
		fmt.Printf("Warning: failed to verify cluster size: %v\n", err)
	} else {
		newSize := updatedHC.Labels["hypershift.openshift.io/hosted-cluster-size"]
		if newSize != targetSize {
			fmt.Printf("Warning: hosted-cluster-size is '%s', expected '%s'\n", newSize, targetSize)
		} else {
			printer.PrintlnGreen(fmt.Sprintf("Verified hosted-cluster-size is now: %s", newSize))
		}
	}

	// Send customer-facing service log
	printer.PrintlnGreen("\nSending customer service log...")
	if err := r.sendCustomerServiceLog(); err != nil {
		fmt.Printf("Warning: failed to send customer service log: %v\n", err)
		fmt.Println("You can send it manually with:")
		fmt.Printf("osdctl servicelog post -C %s -t %s -p INSTANCE_TYPE=%s\n", r.clusterID, resizeRequestServingServiceLogTemplate, targetSize)
	}

	// Print monitoring commands
	fmt.Println("\nResize initiated successfully!")
	fmt.Println("\nUse the following commands to monitor the rollout:")
	fmt.Printf("\nMonitor new nodes being provisioned:\n")
	fmt.Printf("  oc get nodes -l hypershift.openshift.io/cluster-namespace=%s\n", hcNamespace)
	fmt.Printf("\nVerify cluster size (annotation and label):\n")
	fmt.Printf("  oc get hostedcluster -n %s -oyaml | grep -E '(cluster-size-override|hosted-cluster-size)'", hcNamespace)

	return nil
}

func (r *requestServingNodesOpts) findHostedCluster(ctx context.Context, clusterID string) (*hypershiftv1beta1.HostedCluster, error) {
	// Search for the HostedCluster across all namespaces using the label selector
	hostedClusterList := &hypershiftv1beta1.HostedClusterList{}
	listOpts := []client.ListOption{
		client.MatchingLabels{"api.openshift.com/id": clusterID},
	}

	if err := r.mgmtClient.List(ctx, hostedClusterList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list hostedclusters: %v", err)
	}

	if len(hostedClusterList.Items) == 0 {
		return nil, fmt.Errorf("no hostedcluster found with label api.openshift.com/id=%s", clusterID)
	}

	if len(hostedClusterList.Items) > 1 {
		return nil, fmt.Errorf("found %d hostedclusters with label api.openshift.com/id=%s, expected 1", len(hostedClusterList.Items), clusterID)
	}

	return &hostedClusterList.Items[0], nil
}

func (r *requestServingNodesOpts) getAvailableSizes(ctx context.Context) ([]clusterSize, error) {
	// Get the ClusterSizingConfiguration object named "cluster"
	// Use unstructured since the API type may not be fully available
	clusterSizingConfig := &unstructured.Unstructured{}
	clusterSizingConfig.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "scheduling.hypershift.openshift.io",
		Version: "v1alpha1",
		Kind:    "ClusterSizingConfiguration",
	})

	if err := r.mgmtClient.Get(ctx, client.ObjectKey{Name: "cluster"}, clusterSizingConfig); err != nil {
		return nil, fmt.Errorf("failed to get cluster sizing configuration: %v", err)
	}

	// Extract spec.sizes from the unstructured object
	spec, found, err := unstructured.NestedMap(clusterSizingConfig.Object, "spec")
	if err != nil || !found {
		return nil, fmt.Errorf("failed to get spec from cluster sizing configuration: %v", err)
	}

	sizesRaw, found, err := unstructured.NestedSlice(spec, "sizes")
	if err != nil || !found {
		return nil, fmt.Errorf("failed to get sizes from cluster sizing configuration: %v", err)
	}

	var sizes []clusterSize
	for _, sizeRaw := range sizesRaw {
		sizeMap, ok := sizeRaw.(map[string]interface{})
		if !ok {
			continue
		}

		name, _, _ := unstructured.NestedString(sizeMap, "name")
		criteria, _, _ := unstructured.NestedMap(sizeMap, "criteria")

		from, _, _ := unstructured.NestedInt64(criteria, "from")
		to, _, _ := unstructured.NestedInt64(criteria, "to")

		sizes = append(sizes, clusterSize{
			Name: name,
			Criteria: sizeCriteria{
				From: int(from),
				To:   int(to),
			},
		})
	}

	if len(sizes) == 0 {
		return nil, errors.New("no cluster sizes found in configuration")
	}

	return sizes, nil
}

func (r *requestServingNodesOpts) getNextSize(currentSize string, availableSizes []clusterSize) (string, error) {
	currentIndex := -1
	for i, size := range availableSizes {
		if size.Name == currentSize {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 {
		return "", fmt.Errorf("current size '%s' not found in available sizes", currentSize)
	}

	if currentIndex >= len(availableSizes)-1 {
		return "", fmt.Errorf("current size '%s' is already the largest available size", currentSize)
	}

	return availableSizes[currentIndex+1].Name, nil
}

func (r *requestServingNodesOpts) isValidSize(size string, availableSizes []clusterSize) bool {
	for _, s := range availableSizes {
		if s.Name == size {
			return true
		}
	}
	return false
}

func (r *requestServingNodesOpts) getSizeNames(sizes []clusterSize) []string {
	names := make([]string, len(sizes))
	for i, size := range sizes {
		names[i] = size.Name
	}
	return names
}

func (r *requestServingNodesOpts) applyClusterSizeOverride(ctx context.Context, hostedCluster *hypershiftv1beta1.HostedCluster, targetSize string) error {
	// Create a patch to add/update the cluster-size-override annotation
	patch := client.MergeFrom(hostedCluster.DeepCopy())

	// Ensure annotations map exists
	if hostedCluster.Annotations == nil {
		hostedCluster.Annotations = make(map[string]string)
	}

	// Set the override annotation
	hostedCluster.Annotations["hypershift.openshift.io/cluster-size-override"] = targetSize

	// Apply the patch using the admin client (with elevation)
	if err := r.mgmtClientAdmin.Patch(ctx, hostedCluster, patch); err != nil {
		return fmt.Errorf("failed to patch hostedcluster annotation: %v", err)
	}

	return nil
}

func (r *requestServingNodesOpts) sendCustomerServiceLog() error {
	postCmd := servicelog.PostCmdOptions{
		Template:  resizeRequestServingServiceLogTemplate,
		ClusterId: r.clusterID,
	}

	return postCmd.Run()
}
