package cmd

import (
	"context"
	"fmt"
	"slices"
	"time"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

const (
	addonHostingAnnotation  = "addon.open-cluster-management.io/hosting-cluster-name"
	importHostingAnnotation = "import.open-cluster-management.io/hosting-cluster-name"
)

var requiredHostedModeAnnotations = map[string]string{
	"addon.open-cluster-management.io/enable-hosted-mode-addons": "true",
	"cluster.open-cluster-management.io/hypershiftdeployment":    "ignore/ignore",
	"cluster.open-cluster-management.io/provisioner":             "ignore.ignore.HypershiftDeployment.cluster.open-cluster-management.io",
	"import.open-cluster-management.io/klusterlet-deploy-mode":   "Hosted",
	"open-cluster-management/created-via":                        "other",
}

type ReimportRequest struct {
	ClusterID     string
	ReplacementMC string
	Reason        string
	Timeout       time.Duration
}

type ReimportPlan struct {
	ServiceClusterName           string
	ServiceClusterID             string
	ManagedClusterName           string
	CurrentManagementCluster     string
	ReplacementManagementCluster string
	Addons                       []string
	OldAddonManifestWorks        []string

	client         reimportClient
	managedCluster *clusterv1.ManagedCluster
	addons         []addonv1alpha1.ManagedClusterAddOn
	works          []workv1.ManifestWork
}

type ReimportManager interface {
	PrepareReimport(context.Context, ReimportRequest) (ReimportPlan, error)
	Reimport(context.Context, ReimportPlan, time.Duration) error
}

type reimportClient interface {
	internalclient.ManagedClusterClient
	internalclient.ManagedClusterAddonClient
	internalclient.ServiceClusterClient
	OrphanDeleteManifestWork(ctx context.Context, namespace, name, uid string) error
}

type reimportOCMClient interface {
	internalclient.ClusterResolver
	Cluster(identifier string) (*cmv1.Cluster, error)
	ResolveServiceCluster(ctx context.Context, clusterID string) (internalclient.ServiceClusterInfo, error)
	ServiceCluster(clusterID, reason string) (internalclient.ServiceClusterClient, error)
}

type DefaultReimportManager struct{ ocmClient reimportOCMClient }

func NewDefaultReimportManager(ocmClient reimportOCMClient) *DefaultReimportManager {
	return &DefaultReimportManager{ocmClient: ocmClient}
}

func (m *DefaultReimportManager) PrepareReimport(ctx context.Context, request ReimportRequest) (ReimportPlan, error) {
	if request.ClusterID == "" || request.ReplacementMC == "" || request.Reason == "" {
		return ReimportPlan{}, fmt.Errorf("cluster ID, replacement management cluster, and reason are required")
	}
	if request.Timeout <= 0 {
		return ReimportPlan{}, fmt.Errorf("timeout must be positive")
	}
	cluster, err := m.ocmClient.Resolve(ctx, request.ClusterID)
	if err != nil {
		return ReimportPlan{}, fmt.Errorf("resolving cluster: %w", err)
	}
	if cluster.HCPClusterID == "" {
		return ReimportPlan{}, fmt.Errorf("cluster has no canonical HCP ID")
	}
	serviceCluster, err := m.ocmClient.ResolveServiceCluster(ctx, cluster.HCPClusterID)
	if err != nil {
		return ReimportPlan{}, fmt.Errorf("resolving service cluster: %w", err)
	}
	replacement, err := m.ocmClient.Cluster(request.ReplacementMC)
	if err != nil {
		return ReimportPlan{}, fmt.Errorf("resolving replacement management cluster %s: %w", request.ReplacementMC, err)
	}
	if serviceCluster.ID == "" || serviceCluster.Name == "" || replacement.Name() == "" {
		return ReimportPlan{}, fmt.Errorf("service cluster or replacement management cluster is incomplete")
	}
	serviceClient, err := m.ocmClient.ServiceCluster(serviceCluster.ID, request.Reason)
	if err != nil {
		return ReimportPlan{}, fmt.Errorf("connecting to service cluster %s with elevation: %w", serviceCluster.ID, err)
	}
	client, ok := serviceClient.(reimportClient)
	if !ok {
		return ReimportPlan{}, fmt.Errorf("service cluster client does not support ManagedCluster reimport operations")
	}
	managedCluster, err := client.GetManagedCluster(ctx, cluster.HCPClusterID)
	if err != nil {
		return ReimportPlan{}, fmt.Errorf("getting ManagedCluster %s: %w", cluster.HCPClusterID, err)
	}
	if managedCluster.DeletionTimestamp != nil {
		return ReimportPlan{}, fmt.Errorf("ManagedCluster %s is already being deleted", managedCluster.Name)
	}
	current, err := managedClusterHostingCluster(managedCluster.Annotations)
	if err != nil {
		return ReimportPlan{}, fmt.Errorf("validating ManagedCluster %s: %w", managedCluster.Name, err)
	}
	addons, err := client.ListManagedClusterAddons(ctx, managedCluster.Name)
	if err != nil {
		return ReimportPlan{}, fmt.Errorf("listing ManagedClusterAddons in namespace %s: %w", managedCluster.Name, err)
	}
	addonNames := make([]string, 0, len(addons))
	for i := range addons {
		if addons[i].DeletionTimestamp != nil {
			return ReimportPlan{}, fmt.Errorf("ManagedClusterAddon %s/%s is already being deleted", addons[i].Namespace, addons[i].Name)
		}
		if hosting, exists := addons[i].Annotations[addonHostingAnnotation]; exists && hosting != replacement.Name() {
			addonNames = append(addonNames, addons[i].Name)
		}
	}
	works, err := client.ListManifestWorks(ctx, "", internalclient.WithManifestWorkLabel(addonv1alpha1.AddonNamespaceLabelKey, managedCluster.Name))
	if err != nil {
		return ReimportPlan{}, fmt.Errorf("listing old-host addon ManifestWorks: %w", err)
	}
	works = slices.DeleteFunc(works, func(work workv1.ManifestWork) bool {
		return work.Namespace == replacement.Name() || work.Namespace == managedCluster.Name || work.Labels[addonv1alpha1.AddonLabelKey] == ""
	})
	slices.SortFunc(works, func(a, b workv1.ManifestWork) int {
		left, right := a.Namespace+"/"+a.Name, b.Namespace+"/"+b.Name
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
		return 0
	})
	workNames := make([]string, len(works))
	for i := range works {
		if works[i].DeletionTimestamp != nil && (works[i].Spec.DeleteOption == nil || works[i].Spec.DeleteOption.PropagationPolicy != workv1.DeletePropagationPolicyTypeOrphan) {
			return ReimportPlan{}, fmt.Errorf("ManifestWork %s/%s is already deleting without Orphan policy", works[i].Namespace, works[i].Name)
		}
		workNames[i] = works[i].Namespace + "/" + works[i].Name
	}
	return ReimportPlan{
		ServiceClusterName: serviceCluster.Name, ServiceClusterID: serviceCluster.ID,
		ManagedClusterName: managedCluster.Name, CurrentManagementCluster: current, ReplacementManagementCluster: replacement.Name(),
		Addons: addonNames, OldAddonManifestWorks: workNames,
		client: client, managedCluster: managedCluster.DeepCopy(), addons: slices.Clone(addons), works: works,
	}, nil
}

func managedClusterHostingCluster(annotations map[string]string) (string, error) {
	for key, expected := range requiredHostedModeAnnotations {
		if annotations[key] != expected {
			return "", fmt.Errorf("annotation %s is %q, expected %q", key, annotations[key], expected)
		}
	}
	hostingCluster := annotations[importHostingAnnotation]
	if hostingCluster == "" {
		return "", fmt.Errorf("annotation %s must be present", importHostingAnnotation)
	}
	if addonHosting, exists := annotations[addonHostingAnnotation]; exists && addonHosting != hostingCluster {
		return "", fmt.Errorf("hosting annotations are %q and %q, expected matching values", addonHosting, hostingCluster)
	}
	return hostingCluster, nil
}

func (m *DefaultReimportManager) Reimport(ctx context.Context, plan ReimportPlan, timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	managedCluster, err := plan.client.GetManagedCluster(ctx, plan.ManagedClusterName)
	if err != nil {
		return fmt.Errorf("getting ManagedCluster %s before update: %w", plan.ManagedClusterName, err)
	}
	if managedCluster.UID != plan.managedCluster.UID {
		return fmt.Errorf("ManagedCluster %s was recreated; rerun to review it", plan.ManagedClusterName)
	}
	current, err := managedClusterHostingCluster(managedCluster.Annotations)
	if err != nil {
		return fmt.Errorf("validating ManagedCluster %s before update: %w", plan.ManagedClusterName, err)
	}
	if current != plan.CurrentManagementCluster && current != plan.ReplacementManagementCluster {
		return fmt.Errorf("ManagedCluster %s hosting annotation changed from %q to %q", plan.ManagedClusterName, plan.CurrentManagementCluster, current)
	}
	if current != plan.ReplacementManagementCluster {
		_, updateAddonHosting := managedCluster.Annotations[addonHostingAnnotation]
		if err := plan.client.UpdateManagedClusterHosting(ctx, managedCluster.Name, string(plan.managedCluster.UID), managedCluster.ResourceVersion, plan.ReplacementManagementCluster, updateAddonHosting); err != nil {
			return err
		}
	}
	for _, addon := range plan.addons {
		currentHosting, hasHosting := addon.Annotations[addonHostingAnnotation]
		if !hasHosting || currentHosting == plan.ReplacementManagementCluster {
			continue
		}
		if err := plan.client.UpdateManagedClusterAddonHosting(ctx, addon.Namespace, addon.Name, string(addon.UID), currentHosting, plan.ReplacementManagementCluster); err != nil {
			return err
		}
	}
	for _, work := range plan.works {
		if err := plan.client.OrphanDeleteManifestWork(ctx, work.Namespace, work.Name, string(work.UID)); err != nil {
			return fmt.Errorf("orphaning old-host addon ManifestWork after updating hosting annotations: %w", err)
		}
	}
	return waitForOldAddonWorksDeletion(ctx, plan.client, plan.works, time.Now().Add(timeout))
}

func waitForOldAddonWorksDeletion(ctx context.Context, client reimportClient, works []workv1.ManifestWork, deadline time.Time) error {
	for len(works) > 0 {
		remaining := works[:0]
		for _, original := range works {
			work, err := client.GetManifestWork(ctx, original.Namespace, original.Name)
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("waiting for ManifestWork %s/%s deletion: %w", original.Namespace, original.Name, err)
			}
			if work.UID != original.UID {
				return fmt.Errorf("old-host ManifestWork %s/%s was recreated; rerun to review it", work.Namespace, work.Name)
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("timed out waiting for ManifestWork %s/%s deletion; finalizers retained: %v", work.Namespace, work.Name, work.Finalizers)
			}
			remaining = append(remaining, original)
		}
		works = remaining
		if len(works) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for old-host addon ManifestWork deletion: %w", ctx.Err())
		case <-time.After(time.Second):
		}
	}
	return nil
}
