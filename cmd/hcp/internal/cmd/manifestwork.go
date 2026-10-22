package cmd

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"sort"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/retry"
	workv1 "open-cluster-management.io/api/work/v1"
)

const (
	clusterIDLabel            = "api.openshift.com/id"
	managementClusterLabel    = "api.openshift.com/management-cluster"
	hostedClusterLabel        = "api.openshift.com/hosted-cluster"
	nodePoolLabel             = "api.openshift.com/nodepool"
	nodePoolOCMLabel          = "api.openshift.com/nodepool-ocm"
	containsNamespacesLabel   = "containsNamespaces"
	drSecretLabel             = "containsDrSecret"
	veleroAPIVersion          = "velero.io/v1"
	veleroNamespace           = "openshift-adp"
	hostedClusterAPIVersion   = "hypershift.openshift.io/v1beta1"
	hostedClusterKind         = "HostedCluster"
	nodePoolKind              = "NodePool"
	namespaceManifestWorkTail = "-00-namespaces"
)

type ManifestWorkRequest struct {
	ClusterID     string
	SourceMC      string
	ReplacementMC string
	Reason        string
}

// ManifestWorkManager compares and relocates HCP ManifestWorks.
type ManifestWorkManager interface {
	CompareManifestWorks(ctx context.Context, request ManifestWorkRequest) ([]ManifestWorkComparison, error)
	PrepareVeleroManifestWorkMove(ctx context.Context, request ManifestWorkRequest) (ManifestWorkMovePlan, error)
	PrepareClusterManifestWorkMove(ctx context.Context, request ManifestWorkRequest) (ManifestWorkMovePlan, error)
	MoveManifestWorks(ctx context.Context, plan ManifestWorkMovePlan) (ManifestWorkMoveResult, error)
}

// ManifestWorkComparison holds the source and replacement versions of one ManifestWork.
type ManifestWorkComparison struct {
	Name        string
	Source      *workv1.ManifestWork
	Replacement *workv1.ManifestWork
}

// ManifestWorkMovePlan describes the validated changes for one move phase.
type ManifestWorkMovePlan struct {
	Phase              string
	ReplacementMC      string
	ServiceClusterName string
	SourceRetained     bool
	Count              int
	ManifestWorks      []*workv1.ManifestWork

	move        preparedManifestWorkMove
	pause       bool
	sourceWorks []*workv1.ManifestWork
}

// ManifestWorkMoveResult describes an applied move.
type ManifestWorkMoveResult struct {
	Phase         string
	ReplacementMC string
	Created       int
	AlreadyExists bool
}

// DefaultManifestWorkManager is the production ManifestWork implementation.
type DefaultManifestWorkManager struct {
	ocmClient manifestWorkOCMClient
}

type manifestWorkOCMClient interface {
	internalclient.ClusterResolver
	ResolveServiceCluster(ctx context.Context, clusterID string) (internalclient.ServiceClusterInfo, error)
	ServiceCluster(clusterID, reason string) (internalclient.ServiceClusterClient, error)
}

func NewDefaultManifestWorkManager(ocmClient manifestWorkOCMClient) *DefaultManifestWorkManager {
	return &DefaultManifestWorkManager{ocmClient: ocmClient}
}

func (r *DefaultManifestWorkManager) CompareManifestWorks(ctx context.Context, flags ManifestWorkRequest) ([]ManifestWorkComparison, error) {
	cluster, err := r.ocmClient.Resolve(ctx, flags.ClusterID)
	if err != nil {
		return nil, fmt.Errorf("resolving cluster: %w", err)
	}
	sourceMC := sourceManifestWorkNamespace(flags.SourceMC, cluster)
	if sourceMC == "" {
		return nil, fmt.Errorf("source management cluster is empty")
	}
	if sourceMC == flags.ReplacementMC {
		return nil, fmt.Errorf("source and replacement management clusters must differ")
	}
	serviceCluster, err := r.ocmClient.ResolveServiceCluster(ctx, cluster.HCPClusterID)
	if err != nil {
		return nil, fmt.Errorf("resolving service cluster from provision shard: %w", err)
	}
	serviceClient, err := r.ocmClient.ServiceCluster(serviceCluster.ID, flags.Reason)
	if err != nil {
		return nil, fmt.Errorf("connecting to service cluster %s: %w", serviceCluster.ID, err)
	}
	sourceWorks, err := listHCPManifestWorks(ctx, serviceClient, sourceMC, cluster.HCPClusterID)
	if err != nil {
		return nil, err
	}
	replacementWorks, err := listHCPManifestWorks(ctx, serviceClient, flags.ReplacementMC, cluster.HCPClusterID)
	if err != nil {
		return nil, err
	}
	return compareManifestWorks(sourceWorks, replacementWorks), nil
}

func (r *DefaultManifestWorkManager) PrepareVeleroManifestWorkMove(ctx context.Context, flags ManifestWorkRequest) (ManifestWorkMovePlan, error) {
	move, err := r.prepareMove(ctx, flags)
	if err != nil {
		return ManifestWorkMovePlan{}, err
	}
	secretName, schedulesName := disasterRecoveryManifestWorkNames(move.cluster.HCPClusterID)
	works := make([]*workv1.ManifestWork, 0, 2)
	for _, name := range []string{secretName, schedulesName} {
		work, err := getManifestWork(ctx, move.client, move.sourceMC, name)
		if err != nil {
			return ManifestWorkMovePlan{}, err
		}
		if err := validateDRManifestWork(work, move.cluster.HCPClusterID, name == secretName); err != nil {
			return ManifestWorkMovePlan{}, err
		}
		works = append(works, work)
	}
	destinations, err := destinationManifestWorks(move, works, false)
	if err != nil {
		return ManifestWorkMovePlan{}, err
	}
	destinations, err = missingDestinationManifestWorks(ctx, move.client, destinations)
	if err != nil {
		return ManifestWorkMovePlan{}, err
	}
	return ManifestWorkMovePlan{Phase: "Velero resource", ReplacementMC: move.replacementMC, ServiceClusterName: move.serviceCluster.Name, SourceRetained: true, Count: len(destinations), ManifestWorks: destinations, move: move}, nil
}

func (r *DefaultManifestWorkManager) PrepareClusterManifestWorkMove(ctx context.Context, flags ManifestWorkRequest) (ManifestWorkMovePlan, error) {
	move, err := r.prepareMove(ctx, flags)
	if err != nil {
		return ManifestWorkMovePlan{}, err
	}
	if err := requireDestinationVeleroResources(ctx, move.client, move.replacementMC, move.cluster.HCPClusterID); err != nil {
		return ManifestWorkMovePlan{}, err
	}
	works, err := clusterManifestWorks(ctx, move.client, move.sourceMC, move.cluster.HCPClusterID)
	if err != nil {
		return ManifestWorkMovePlan{}, err
	}
	expectedDestinations, err := destinationManifestWorks(move, works, true)
	if err != nil {
		return ManifestWorkMovePlan{}, err
	}
	destinations, err := missingDestinationManifestWorks(ctx, move.client, expectedDestinations)
	if err != nil {
		return ManifestWorkMovePlan{}, err
	}
	return ManifestWorkMovePlan{Phase: "cluster resource", ReplacementMC: move.replacementMC, ServiceClusterName: move.serviceCluster.Name, SourceRetained: true, Count: len(works), ManifestWorks: destinations, move: move, pause: true, sourceWorks: works}, nil
}

type preparedManifestWorkMove struct {
	cluster        internalclient.ClusterInfo
	sourceMC       string
	replacementMC  string
	serviceCluster internalclient.ServiceClusterInfo
	client         internalclient.ServiceClusterClient
}

func (r *DefaultManifestWorkManager) prepareMove(ctx context.Context, flags ManifestWorkRequest) (preparedManifestWorkMove, error) {
	cluster, err := r.ocmClient.Resolve(ctx, flags.ClusterID)
	if err != nil {
		return preparedManifestWorkMove{}, fmt.Errorf("resolving cluster: %w", err)
	}
	sourceMC := sourceManifestWorkNamespace(flags.SourceMC, cluster)
	if sourceMC == "" {
		return preparedManifestWorkMove{}, fmt.Errorf("source management cluster is empty")
	}
	if sourceMC == flags.ReplacementMC {
		return preparedManifestWorkMove{}, fmt.Errorf("source and replacement management clusters must differ")
	}
	serviceCluster, err := r.ocmClient.ResolveServiceCluster(ctx, cluster.HCPClusterID)
	if err != nil {
		return preparedManifestWorkMove{}, fmt.Errorf("resolving service cluster from provision shard: %w", err)
	}
	serviceClient, err := r.ocmClient.ServiceCluster(serviceCluster.ID, flags.Reason)
	if err != nil {
		return preparedManifestWorkMove{}, fmt.Errorf("connecting to service cluster %s with elevation: %w", serviceCluster.ID, err)
	}
	return preparedManifestWorkMove{cluster: cluster, sourceMC: sourceMC, replacementMC: flags.ReplacementMC, serviceCluster: serviceCluster, client: serviceClient}, nil
}

func (r *DefaultManifestWorkManager) MoveManifestWorks(ctx context.Context, plan ManifestWorkMovePlan) (ManifestWorkMoveResult, error) {
	destinations, err := missingDestinationManifestWorks(ctx, plan.move.client, plan.ManifestWorks)
	if err != nil {
		return ManifestWorkMoveResult{}, err
	}
	if plan.pause {
		for _, work := range plan.sourceWorks {
			if work.Name == plan.move.cluster.HCPClusterID || isNodePoolManifestWork(work) {
				if err := pauseManifestWork(ctx, plan.move.client, work); err != nil {
					return ManifestWorkMoveResult{}, err
				}
			}
		}
	}
	if len(destinations) == 0 {
		return ManifestWorkMoveResult{Phase: plan.Phase, ReplacementMC: plan.ReplacementMC, AlreadyExists: true}, nil
	}
	for _, destination := range destinations {
		if err := plan.move.client.CreateManifestWork(ctx, destination); err != nil {
			return ManifestWorkMoveResult{}, fmt.Errorf("creating destination ManifestWork %s/%s: %w", destination.Namespace, destination.Name, err)
		}
		verified, err := plan.move.client.GetManifestWork(ctx, destination.Namespace, destination.Name)
		if err != nil {
			return ManifestWorkMoveResult{}, fmt.Errorf("verifying destination ManifestWork %s/%s: %w", destination.Namespace, destination.Name, err)
		}
		if !manifestWorksMatch(destination, verified) {
			return ManifestWorkMoveResult{}, fmt.Errorf("destination ManifestWork %s/%s has unexpected content", destination.Namespace, destination.Name)
		}
	}
	return ManifestWorkMoveResult{Phase: plan.Phase, ReplacementMC: plan.ReplacementMC, Created: len(destinations)}, nil
}

func destinationManifestWorks(move preparedManifestWorkMove, works []*workv1.ManifestWork, pause bool) ([]*workv1.ManifestWork, error) {
	destinations := make([]*workv1.ManifestWork, 0, len(works))
	for _, work := range works {
		source := work
		if pause && (work.Name == move.cluster.HCPClusterID || isNodePoolManifestWork(work)) {
			source = work.DeepCopy()
			if err := pauseManifestWorkPayload(source); err != nil {
				return nil, err
			}
		}
		destination := sanitizeManifestWork(source, move.replacementMC)
		destination.Labels[managementClusterLabel] = move.replacementMC
		destinations = append(destinations, destination)
	}
	return destinations, nil
}

func missingDestinationManifestWorks(ctx context.Context, serviceClient internalclient.ServiceClusterClient, destinations []*workv1.ManifestWork) ([]*workv1.ManifestWork, error) {
	missing := make([]*workv1.ManifestWork, 0, len(destinations))
	for _, destination := range destinations {
		existing, err := serviceClient.GetManifestWork(ctx, destination.Namespace, destination.Name)
		switch {
		case apierrors.IsNotFound(err):
			missing = append(missing, destination)
		case err != nil:
			return nil, fmt.Errorf("checking destination ManifestWork %s/%s: %w", destination.Namespace, destination.Name, err)
		case !manifestWorksMatch(destination, existing):
			return nil, fmt.Errorf("destination ManifestWork %s/%s already exists with different content", destination.Namespace, destination.Name)
		}
	}
	return missing, nil
}

func sourceManifestWorkNamespace(source string, cluster internalclient.ClusterInfo) string {
	if source != "" {
		return source
	}
	return cluster.MgmtClusterName
}

func listManifestWorks(ctx context.Context, serviceClient internalclient.ServiceClusterClient, namespace string) ([]workv1.ManifestWork, error) {
	works, err := serviceClient.ListManifestWorks(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("listing ManifestWorks in namespace %s: %w", namespace, err)
	}
	return works, nil
}

func listHCPManifestWorks(ctx context.Context, serviceClient internalclient.ServiceClusterClient, namespace, clusterID string) ([]workv1.ManifestWork, error) {
	works, err := serviceClient.ListManifestWorks(ctx, namespace, internalclient.WithManifestWorkLabel(clusterIDLabel, clusterID))
	if err != nil {
		return nil, fmt.Errorf("listing ManifestWorks for cluster %s in namespace %s: %w", clusterID, namespace, err)
	}
	return works, nil
}

func compareManifestWorks(sourceWorks, replacementWorks []workv1.ManifestWork) []ManifestWorkComparison {
	byName := make(map[string]ManifestWorkComparison, len(sourceWorks)+len(replacementWorks))
	for i := range sourceWorks {
		comparison := byName[sourceWorks[i].Name]
		comparison.Name = sourceWorks[i].Name
		comparison.Source = &sourceWorks[i]
		byName[comparison.Name] = comparison
	}
	for i := range replacementWorks {
		comparison := byName[replacementWorks[i].Name]
		comparison.Name = replacementWorks[i].Name
		comparison.Replacement = &replacementWorks[i]
		byName[comparison.Name] = comparison
	}
	comparisons := make([]ManifestWorkComparison, 0, len(byName))
	for _, comparison := range byName {
		comparisons = append(comparisons, comparison)
	}
	sort.Slice(comparisons, func(i, j int) bool { return comparisons[i].Name < comparisons[j].Name })
	return comparisons
}

func presenceStatus(work *workv1.ManifestWork) string {
	if work == nil {
		return "No"
	}
	return "Yes"
}

func conditionStatusForWork(work *workv1.ManifestWork, conditionType string) string {
	if work == nil {
		return "-"
	}
	return conditionStatus(work.Status.Conditions, conditionType)
}

func getManifestWork(ctx context.Context, serviceClient internalclient.ServiceClusterClient, namespace, name string) (*workv1.ManifestWork, error) {
	work, err := serviceClient.GetManifestWork(ctx, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("getting source ManifestWork %s/%s: %w", namespace, name, err)
	}
	if work.DeletionTimestamp != nil {
		return nil, fmt.Errorf("source ManifestWork %s/%s is being deleted", namespace, name)
	}
	return work, nil
}

func disasterRecoveryManifestWorkNames(clusterID string) (string, string) {
	return clusterID + "-01-dr", clusterID + "-00-dr"
}

func requireDestinationVeleroResources(ctx context.Context, serviceClient internalclient.ServiceClusterClient, namespace, clusterID string) error {
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	for _, name := range []string{secretName, schedulesName} {
		work, err := serviceClient.GetManifestWork(ctx, namespace, name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("destination Velero resource ManifestWork %s/%s is required before moving cluster resources", namespace, name)
			}
			return fmt.Errorf("getting destination Velero resource ManifestWork %s/%s: %w", namespace, name, err)
		}
		if err := validateDRManifestWork(work, clusterID, name == secretName); err != nil {
			return fmt.Errorf("validating destination Velero resource ManifestWork: %w", err)
		}
	}
	return nil
}

func validateDRManifestWork(work *workv1.ManifestWork, clusterID string, secretWork bool) error {
	if work.Labels[clusterIDLabel] != clusterID {
		return fmt.Errorf("ManifestWork %s/%s does not belong to cluster %s", work.Namespace, work.Name, clusterID)
	}
	if secretWork && work.Labels[drSecretLabel] != "true" {
		return fmt.Errorf("ManifestWork %s/%s is not the disaster recovery secret work", work.Namespace, work.Name)
	}
	if len(work.Spec.Workload.Manifests) == 0 {
		return fmt.Errorf("ManifestWork %s/%s has no manifests", work.Namespace, work.Name)
	}
	for i := range work.Spec.Workload.Manifests {
		object, err := manifestObject(&work.Spec.Workload.Manifests[i])
		if err != nil {
			return fmt.Errorf("decoding ManifestWork %s/%s: %w", work.Namespace, work.Name, err)
		}
		if object.GetNamespace() != veleroNamespace {
			return fmt.Errorf("ManifestWork %s/%s contains an object outside %s", work.Namespace, work.Name, veleroNamespace)
		}
		if secretWork {
			if object.GetAPIVersion() != "v1" || object.GetKind() != "Secret" || object.GetName() != clusterID+"-dr-creds" {
				return fmt.Errorf("ManifestWork %s/%s has an unexpected disaster recovery secret", work.Namespace, work.Name)
			}
			continue
		}
		if object.GetAPIVersion() != veleroAPIVersion || (object.GetKind() != "Schedule" && object.GetKind() != "BackupStorageLocation") {
			return fmt.Errorf("ManifestWork %s/%s has an unexpected disaster recovery resource", work.Namespace, work.Name)
		}
	}
	return nil
}

func clusterManifestWorks(ctx context.Context, serviceClient internalclient.ServiceClusterClient, namespace, clusterID string) ([]*workv1.ManifestWork, error) {
	namespaceWork, err := getManifestWork(ctx, serviceClient, namespace, clusterID+namespaceManifestWorkTail)
	if err != nil {
		return nil, err
	}
	if namespaceWork.Labels[clusterIDLabel] != clusterID || namespaceWork.Labels[containsNamespacesLabel] != "true" {
		return nil, fmt.Errorf("ManifestWork %s/%s is not the cluster namespace work", namespace, namespaceWork.Name)
	}
	hostedClusterWork, err := getManifestWork(ctx, serviceClient, namespace, clusterID)
	if err != nil {
		return nil, err
	}
	if hostedClusterWork.Labels[clusterIDLabel] != clusterID || hostedClusterWork.Labels[hostedClusterLabel] != "true" {
		return nil, fmt.Errorf("ManifestWork %s/%s is not the HostedCluster work", namespace, hostedClusterWork.Name)
	}
	if _, err := pauseableManifestIndex(hostedClusterWork, hostedClusterAPIVersion, hostedClusterKind); err != nil {
		return nil, err
	}
	allWorks, err := listManifestWorks(ctx, serviceClient, namespace)
	if err != nil {
		return nil, err
	}
	nodePoolWorks := make([]*workv1.ManifestWork, 0)
	for i := range allWorks {
		work := &allWorks[i]
		if work.Labels[clusterIDLabel] != clusterID || !isNodePoolManifestWork(work) {
			continue
		}
		if _, err := pauseableManifestIndex(work, hostedClusterAPIVersion, nodePoolKind); err != nil {
			return nil, err
		}
		nodePoolWorks = append(nodePoolWorks, work)
	}
	sort.Slice(nodePoolWorks, func(i, j int) bool { return nodePoolWorks[i].Name < nodePoolWorks[j].Name })
	works := make([]*workv1.ManifestWork, 0, len(nodePoolWorks)+2)
	works = append(works, namespaceWork, hostedClusterWork)
	works = append(works, nodePoolWorks...)
	return works, nil
}

func isNodePoolManifestWork(work *workv1.ManifestWork) bool {
	_, current := work.Labels[nodePoolOCMLabel]
	_, legacy := work.Labels[nodePoolLabel]
	return current || legacy
}

func pauseManifestWork(ctx context.Context, serviceClient internalclient.ServiceClusterClient, work *workv1.ManifestWork) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := getManifestWork(ctx, serviceClient, work.Namespace, work.Name)
		if err != nil {
			return err
		}
		if err := pauseManifestWorkPayload(current); err != nil {
			return err
		}
		if err := serviceClient.UpdateManifestWork(ctx, current); err != nil {
			return fmt.Errorf("updating paused ManifestWork %s/%s: %w", current.Namespace, current.Name, err)
		}
		*work = *current
		return nil
	})
}

func pauseManifestWorkPayload(work *workv1.ManifestWork) error {
	apiVersion, kind := hostedClusterAPIVersion, hostedClusterKind
	if isNodePoolManifestWork(work) {
		kind = nodePoolKind
	}
	index, err := pauseableManifestIndex(work, apiVersion, kind)
	if err != nil {
		return err
	}
	object, err := manifestObject(&work.Spec.Workload.Manifests[index])
	if err != nil {
		return fmt.Errorf("decoding ManifestWork %s/%s: %w", work.Namespace, work.Name, err)
	}
	if err := unstructured.SetNestedField(object.Object, "true", "spec", "pausedUntil"); err != nil {
		return fmt.Errorf("pausing %s in ManifestWork %s/%s: %w", kind, work.Namespace, work.Name, err)
	}
	raw, err := json.Marshal(object.Object)
	if err != nil {
		return fmt.Errorf("encoding paused %s in ManifestWork %s/%s: %w", kind, work.Namespace, work.Name, err)
	}
	work.Spec.Workload.Manifests[index].Raw = raw
	work.Spec.Workload.Manifests[index].Object = nil
	return nil
}

func pauseableManifestIndex(work *workv1.ManifestWork, apiVersion, kind string) (int, error) {
	found := -1
	for i := range work.Spec.Workload.Manifests {
		object, err := manifestObject(&work.Spec.Workload.Manifests[i])
		if err != nil {
			return 0, fmt.Errorf("decoding ManifestWork %s/%s: %w", work.Namespace, work.Name, err)
		}
		if object.GetAPIVersion() != apiVersion || object.GetKind() != kind {
			continue
		}
		if found >= 0 {
			return 0, fmt.Errorf("ManifestWork %s/%s contains multiple %s manifests", work.Namespace, work.Name, kind)
		}
		found = i
	}
	if found < 0 {
		return 0, fmt.Errorf("ManifestWork %s/%s does not contain a %s manifest", work.Namespace, work.Name, kind)
	}
	return found, nil
}

func manifestObject(manifest *workv1.Manifest) (*unstructured.Unstructured, error) {
	raw := manifest.Raw
	if len(raw) == 0 && manifest.Object != nil {
		var err error
		raw, err = json.Marshal(manifest.Object)
		if err != nil {
			return nil, err
		}
	}
	object := &unstructured.Unstructured{}
	if err := json.Unmarshal(raw, &object.Object); err != nil {
		return nil, err
	}
	return object, nil
}

func sanitizeManifestWork(source *workv1.ManifestWork, namespace string) *workv1.ManifestWork {
	destination := source.DeepCopy()
	destination.Namespace = namespace
	destination.UID = ""
	destination.ResourceVersion = ""
	destination.Generation = 0
	destination.CreationTimestamp = metav1.Time{}
	destination.DeletionTimestamp = nil
	destination.DeletionGracePeriodSeconds = nil
	destination.ManagedFields = nil
	destination.Finalizers = nil
	destination.OwnerReferences = nil
	destination.Status = workv1.ManifestWorkStatus{}
	if destination.Labels == nil {
		destination.Labels = map[string]string{}
	}
	return destination
}

func manifestWorksMatch(expected, actual *workv1.ManifestWork) bool {
	return expected.Name == actual.Name &&
		expected.Namespace == actual.Namespace &&
		equality.Semantic.DeepEqual(expected.Labels, actual.Labels) &&
		equality.Semantic.DeepEqual(expected.Annotations, actual.Annotations) &&
		manifestWorkSpecsMatch(expected.Spec, actual.Spec)
}

func manifestWorkSpecsMatch(expected, actual workv1.ManifestWorkSpec) bool {
	if !equality.Semantic.DeepEqual(expected.DeleteOption, actual.DeleteOption) ||
		!equality.Semantic.DeepEqual(expected.ManifestConfigs, actual.ManifestConfigs) ||
		len(expected.Workload.Manifests) != len(actual.Workload.Manifests) {
		return false
	}
	for i := range expected.Workload.Manifests {
		if !manifestMatches(&expected.Workload.Manifests[i], &actual.Workload.Manifests[i]) {
			return false
		}
	}
	return true
}

func manifestMatches(expected, actual *workv1.Manifest) bool {
	expectedObject, err := manifestObject(expected)
	if err != nil {
		return false
	}
	actualObject, err := manifestObject(actual)
	if err != nil {
		return false
	}
	expectedJSON, err := json.Marshal(expectedObject.Object)
	if err != nil {
		return false
	}
	actualJSON, err := json.Marshal(actualObject.Object)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(expectedJSON, actualJSON) == 1
}

func conditionStatus(conditions []metav1.Condition, conditionType string) string {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return string(condition.Status)
		}
	}
	return "Unknown"
}
