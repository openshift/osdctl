package backup

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

type manifestWorkCompareFlags struct {
	clusterID     string
	sourceMC      string
	replacementMC string
	reason        string
}

type manifestWorkMoveFlags struct {
	clusterID     string
	sourceMC      string
	replacementMC string
	reason        string
}

type manifestWorkRunner struct {
	resolver              ClusterResolver
	builder               KubeClientBuilder
	resolveServiceCluster func(context.Context, string) (ServiceClusterInfo, error)
	in                    io.Reader
	out                   io.Writer
}

func newCmdManifestWork() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "manifestwork",
		Short:             "Compare and relocate HCP ManifestWorks",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}
	cmd.AddCommand(newCmdManifestWorkCompare())
	cmd.AddCommand(newCmdManifestWorkMove())
	return cmd
}

func newCmdManifestWorkCompare() *cobra.Command {
	flags := &manifestWorkCompareFlags{}
	cmd := &cobra.Command{
		Use:   "compare --cluster-id <cluster-id> --replacement-mc <management-cluster> --reason <reason> [--source-mc <management-cluster>]",
		Short: "Compare HCP ManifestWorks between management clusters",
		Example: "  osdctl hcp backup manifestwork compare --cluster-id ${CLUSTER_ID} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}\n" +
			"  osdctl hcp backup manifestwork compare --cluster-id ${CLUSTER_ID} --source-mc ${SOURCE_MC} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManifestWorkCommand(cmd, func(r *manifestWorkRunner) error {
				return r.compare(cmd.Context(), *flags)
			})
		},
	}
	cmd.Flags().StringVarP(&flags.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().StringVar(&flags.sourceMC, "source-mc", "", "Management cluster whose namespace contains the ManifestWorks (defaults to the HCP management cluster)")
	cmd.Flags().StringVar(&flags.replacementMC, "replacement-mc", "", "Management cluster whose namespace contains the replacement ManifestWorks")
	cmd.Flags().StringVar(&flags.reason, "reason", "", "Reason for elevated access")
	for _, flag := range []string{"cluster-id", "replacement-mc", "reason"} {
		_ = cmd.MarkFlagRequired(flag)
	}
	return cmd
}

func newCmdManifestWorkMove() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "move",
		Short:             "Move HCP Velero and cluster resources in phases",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}
	cmd.AddCommand(newCmdManifestWorkMoveVeleroResources())
	cmd.AddCommand(newCmdManifestWorkMoveClusterResources())
	return cmd
}

func newCmdManifestWorkMoveVeleroResources() *cobra.Command {
	flags := &manifestWorkMoveFlags{}
	cmd := newManifestWorkMoveCommand("velero-resources", "Move the Velero resource ManifestWorks", flags)
	cmd.Long = "Copy the exact Velero credential secret and schedule ManifestWorks to the replacement management cluster. Source ManifestWorks are retained."
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runManifestWorkCommand(cmd, func(r *manifestWorkRunner) error {
			return r.moveVeleroResources(cmd.Context(), *flags)
		})
	}
	return cmd
}

func newCmdManifestWorkMoveClusterResources() *cobra.Command {
	flags := &manifestWorkMoveFlags{}
	cmd := newManifestWorkMoveCommand("cluster-resources", "Move the namespace, HostedCluster, and NodePool ManifestWorks", flags)
	cmd.Long = "After the Velero resource ManifestWorks are present in the replacement management cluster, pause the source HostedCluster and NodePool payloads and copy the namespace, HostedCluster, and NodePool ManifestWorks. Source ManifestWorks are retained and both source and destination payloads remain paused."
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runManifestWorkCommand(cmd, func(r *manifestWorkRunner) error {
			return r.moveClusterResources(cmd.Context(), *flags)
		})
	}
	return cmd
}

func newManifestWorkMoveCommand(use, short string, flags *manifestWorkMoveFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use + " --cluster-id <cluster-id> --replacement-mc <management-cluster> --reason <reason> [--source-mc <management-cluster>]",
		Short: short,
		Example: "  osdctl hcp backup manifestwork move " + use + " --cluster-id ${CLUSTER_ID} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}\n" +
			"  osdctl hcp backup manifestwork move " + use + " --cluster-id ${CLUSTER_ID} --source-mc ${SOURCE_MC} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}
	cmd.Flags().StringVarP(&flags.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().StringVar(&flags.sourceMC, "source-mc", "", "Management cluster whose namespace contains the source ManifestWorks (defaults to the HCP management cluster)")
	cmd.Flags().StringVar(&flags.replacementMC, "replacement-mc", "", "Management cluster whose namespace will contain the replacement ManifestWorks")
	cmd.Flags().StringVar(&flags.reason, "reason", "", "Reason for elevated access")
	for _, flag := range []string{"cluster-id", "replacement-mc", "reason"} {
		_ = cmd.MarkFlagRequired(flag)
	}
	return cmd
}

func runManifestWorkCommand(cmd *cobra.Command, run func(*manifestWorkRunner) error) error {
	ocmConn, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("creating OCM connection: %w", err)
	}
	defer ocmConn.Close()
	return run(newManifestWorkRunner(ocmConn, cmd.InOrStdin(), cmd.OutOrStdout()))
}

func newManifestWorkRunner(ocmConn *ocmsdk.Connection, in io.Reader, out io.Writer) *manifestWorkRunner {
	logger := logrus.New()
	return &manifestWorkRunner{
		resolver: &ocmClusterResolver{ocmConn: ocmConn, logger: logger},
		builder:  &backplaneClientBuilder{ocmConn: ocmConn, logger: logger},
		resolveServiceCluster: func(ctx context.Context, clusterID string) (ServiceClusterInfo, error) {
			return resolveServiceCluster(ctx, ocmConn, clusterID)
		},
		in:  in,
		out: out,
	}
}

func (r *manifestWorkRunner) compare(ctx context.Context, flags manifestWorkCompareFlags) error {
	cluster, err := r.resolver.Resolve(ctx, flags.clusterID)
	if err != nil {
		return fmt.Errorf("resolving cluster: %w", err)
	}
	sourceMC := sourceManifestWorkNamespace(flags.sourceMC, cluster)
	if sourceMC == "" {
		return fmt.Errorf("source management cluster is empty")
	}
	if sourceMC == flags.replacementMC {
		return fmt.Errorf("source and replacement management clusters must differ")
	}
	serviceCluster, err := r.resolveServiceCluster(ctx, cluster.HCPClusterID)
	if err != nil {
		return fmt.Errorf("resolving service cluster from provision shard: %w", err)
	}
	serviceClient, err := r.builder.Build(ctx, WithClusterID{ClusterID: serviceCluster.ID}, WithElevation{Reason: flags.reason})
	if err != nil {
		return fmt.Errorf("connecting to service cluster %s: %w", serviceCluster.ID, err)
	}
	sourceWorks, err := listHCPManifestWorks(ctx, serviceClient, sourceMC, cluster.HCPClusterID)
	if err != nil {
		return err
	}
	replacementWorks, err := listHCPManifestWorks(ctx, serviceClient, flags.replacementMC, cluster.HCPClusterID)
	if err != nil {
		return err
	}
	works := compareManifestWorks(sourceWorks, replacementWorks)
	table := printer.NewTablePrinter(r.out, 0, 4, 2, ' ')
	table.AddRow([]string{"NAME", "SOURCE", "SOURCE APPLIED", "SOURCE AVAILABLE", "REPLACEMENT", "REPLACEMENT APPLIED", "REPLACEMENT AVAILABLE"})
	for _, work := range works {
		table.AddRow([]string{work.name, presenceStatus(work.source), conditionStatusForWork(work.source, workv1.WorkApplied), conditionStatusForWork(work.source, workv1.WorkAvailable), presenceStatus(work.replacement), conditionStatusForWork(work.replacement, workv1.WorkApplied), conditionStatusForWork(work.replacement, workv1.WorkAvailable)})
	}
	return table.Flush()
}

func (r *manifestWorkRunner) moveVeleroResources(ctx context.Context, flags manifestWorkMoveFlags) error {
	move, err := r.prepareMove(ctx, flags)
	if err != nil {
		return err
	}
	secretName, schedulesName := disasterRecoveryManifestWorkNames(move.cluster.HCPClusterID)
	works := make([]*workv1.ManifestWork, 0, 2)
	for _, name := range []string{secretName, schedulesName} {
		work, err := getManifestWork(ctx, move.client, move.sourceMC, name)
		if err != nil {
			return err
		}
		if err := validateDRManifestWork(work, move.cluster.HCPClusterID, name == secretName); err != nil {
			return err
		}
		works = append(works, work)
	}
	return r.copyManifestWorks(ctx, move, works, "Velero resource", true)
}

func (r *manifestWorkRunner) moveClusterResources(ctx context.Context, flags manifestWorkMoveFlags) error {
	move, err := r.prepareMove(ctx, flags)
	if err != nil {
		return err
	}
	if err := requireDestinationVeleroResources(ctx, move.client, move.replacementMC, move.cluster.HCPClusterID); err != nil {
		return err
	}
	works, err := clusterManifestWorks(ctx, move.client, move.sourceMC, move.cluster.HCPClusterID)
	if err != nil {
		return err
	}
	expectedDestinations, err := destinationManifestWorks(move, works, true)
	if err != nil {
		return err
	}
	if _, err := missingDestinationManifestWorks(ctx, move.client, expectedDestinations); err != nil {
		return err
	}
	if !r.confirmClusterMove(move, len(works)) {
		return nil
	}
	for _, work := range works {
		if work.Name == move.cluster.HCPClusterID || isNodePoolManifestWork(work) {
			if err := pauseManifestWork(ctx, move.client, work); err != nil {
				return err
			}
		}
	}
	return r.copyManifestWorks(ctx, move, works, "cluster resource", false)
}

type preparedManifestWorkMove struct {
	cluster        ClusterInfo
	sourceMC       string
	replacementMC  string
	serviceCluster ServiceClusterInfo
	client         KubeClient
}

func (r *manifestWorkRunner) prepareMove(ctx context.Context, flags manifestWorkMoveFlags) (preparedManifestWorkMove, error) {
	cluster, err := r.resolver.Resolve(ctx, flags.clusterID)
	if err != nil {
		return preparedManifestWorkMove{}, fmt.Errorf("resolving cluster: %w", err)
	}
	sourceMC := sourceManifestWorkNamespace(flags.sourceMC, cluster)
	if sourceMC == "" {
		return preparedManifestWorkMove{}, fmt.Errorf("source management cluster is empty")
	}
	if sourceMC == flags.replacementMC {
		return preparedManifestWorkMove{}, fmt.Errorf("source and replacement management clusters must differ")
	}
	serviceCluster, err := r.resolveServiceCluster(ctx, cluster.HCPClusterID)
	if err != nil {
		return preparedManifestWorkMove{}, fmt.Errorf("resolving service cluster from provision shard: %w", err)
	}
	serviceClient, err := r.builder.Build(ctx, WithClusterID{ClusterID: serviceCluster.ID}, WithElevation{Reason: flags.reason})
	if err != nil {
		return preparedManifestWorkMove{}, fmt.Errorf("connecting to service cluster %s with elevation: %w", serviceCluster.ID, err)
	}
	return preparedManifestWorkMove{cluster: cluster, sourceMC: sourceMC, replacementMC: flags.replacementMC, serviceCluster: serviceCluster, client: serviceClient}, nil
}

func (r *manifestWorkRunner) copyManifestWorks(ctx context.Context, move preparedManifestWorkMove, works []*workv1.ManifestWork, phase string, confirm bool) error {
	destinations, err := destinationManifestWorks(move, works, false)
	if err != nil {
		return err
	}
	destinations, err = missingDestinationManifestWorks(ctx, move.client, destinations)
	if err != nil {
		return err
	}
	if len(destinations) == 0 {
		fmt.Fprintf(r.out, "All %s ManifestWorks already exist in %s. Source ManifestWorks were retained.\n", phase, move.replacementMC)
		return nil
	}
	if confirm {
		fmt.Fprintf(r.out, "Create %d %s ManifestWork(s) in %s on service cluster %s?\nSource ManifestWorks will be retained. [y/N] ", len(destinations), phase, move.replacementMC, move.serviceCluster.Name)
		if !confirmManifestWorkMove(r.in) {
			fmt.Fprintln(r.out, "ManifestWork move cancelled.")
			return nil
		}
	}
	for _, destination := range destinations {
		if err := move.client.Create(ctx, destination); err != nil {
			return fmt.Errorf("creating destination ManifestWork %s/%s: %w", destination.Namespace, destination.Name, err)
		}
		verified := &workv1.ManifestWork{}
		if err := move.client.Get(ctx, client.ObjectKeyFromObject(destination), verified); err != nil {
			return fmt.Errorf("verifying destination ManifestWork %s/%s: %w", destination.Namespace, destination.Name, err)
		}
		if !manifestWorksMatch(destination, verified) {
			return fmt.Errorf("destination ManifestWork %s/%s has unexpected content", destination.Namespace, destination.Name)
		}
	}
	fmt.Fprintf(r.out, "Created %d %s ManifestWork(s) in %s. Source ManifestWorks were retained.\n", len(destinations), phase, move.replacementMC)
	return nil
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

func missingDestinationManifestWorks(ctx context.Context, serviceClient KubeClient, destinations []*workv1.ManifestWork) ([]*workv1.ManifestWork, error) {
	missing := make([]*workv1.ManifestWork, 0, len(destinations))
	for _, destination := range destinations {
		existing := &workv1.ManifestWork{}
		err := serviceClient.Get(ctx, client.ObjectKeyFromObject(destination), existing)
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

func (r *manifestWorkRunner) confirmClusterMove(move preparedManifestWorkMove, count int) bool {
	fmt.Fprintf(r.out, "Pause HostedCluster and NodePool reconciliation in the source, then move %d cluster ManifestWork(s) to %s on service cluster %s?\nSource ManifestWorks will be retained and both source and destination resources will remain paused. [y/N] ", count, move.replacementMC, move.serviceCluster.Name)
	if confirmManifestWorkMove(r.in) {
		return true
	}
	fmt.Fprintln(r.out, "ManifestWork move cancelled.")
	return false
}

func sourceManifestWorkNamespace(source string, cluster ClusterInfo) string {
	if source != "" {
		return source
	}
	return cluster.MgmtClusterName
}

func listManifestWorks(ctx context.Context, serviceClient KubeClient, namespace string) ([]workv1.ManifestWork, error) {
	list := &workv1.ManifestWorkList{}
	if err := serviceClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("listing ManifestWorks in namespace %s: %w", namespace, err)
	}
	sort.Slice(list.Items, func(i, j int) bool { return list.Items[i].Name < list.Items[j].Name })
	return list.Items, nil
}

func listHCPManifestWorks(ctx context.Context, serviceClient KubeClient, namespace, clusterID string) ([]workv1.ManifestWork, error) {
	list := &workv1.ManifestWorkList{}
	if err := serviceClient.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{clusterIDLabel: clusterID}); err != nil {
		return nil, fmt.Errorf("listing ManifestWorks for cluster %s in namespace %s: %w", clusterID, namespace, err)
	}
	sort.Slice(list.Items, func(i, j int) bool { return list.Items[i].Name < list.Items[j].Name })
	return list.Items, nil
}

type manifestWorkComparison struct {
	name        string
	source      *workv1.ManifestWork
	replacement *workv1.ManifestWork
}

func compareManifestWorks(sourceWorks, replacementWorks []workv1.ManifestWork) []manifestWorkComparison {
	byName := make(map[string]manifestWorkComparison, len(sourceWorks)+len(replacementWorks))
	for i := range sourceWorks {
		comparison := byName[sourceWorks[i].Name]
		comparison.name = sourceWorks[i].Name
		comparison.source = &sourceWorks[i]
		byName[comparison.name] = comparison
	}
	for i := range replacementWorks {
		comparison := byName[replacementWorks[i].Name]
		comparison.name = replacementWorks[i].Name
		comparison.replacement = &replacementWorks[i]
		byName[comparison.name] = comparison
	}
	comparisons := make([]manifestWorkComparison, 0, len(byName))
	for _, comparison := range byName {
		comparisons = append(comparisons, comparison)
	}
	sort.Slice(comparisons, func(i, j int) bool { return comparisons[i].name < comparisons[j].name })
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

func getManifestWork(ctx context.Context, serviceClient KubeClient, namespace, name string) (*workv1.ManifestWork, error) {
	work := &workv1.ManifestWork{}
	if err := serviceClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, work); err != nil {
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

func requireDestinationVeleroResources(ctx context.Context, serviceClient KubeClient, namespace, clusterID string) error {
	secretName, schedulesName := disasterRecoveryManifestWorkNames(clusterID)
	for _, name := range []string{secretName, schedulesName} {
		work := &workv1.ManifestWork{}
		if err := serviceClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, work); err != nil {
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

func clusterManifestWorks(ctx context.Context, serviceClient KubeClient, namespace, clusterID string) ([]*workv1.ManifestWork, error) {
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

func pauseManifestWork(ctx context.Context, serviceClient KubeClient, work *workv1.ManifestWork) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := getManifestWork(ctx, serviceClient, work.Namespace, work.Name)
		if err != nil {
			return err
		}
		if err := pauseManifestWorkPayload(current); err != nil {
			return err
		}
		if err := serviceClient.Update(ctx, current); err != nil {
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

func confirmManifestWorkMove(in io.Reader) bool {
	var response string
	if _, err := fmt.Fscanln(in, &response); err != nil {
		return false
	}
	return strings.EqualFold(response, "y") || strings.EqualFold(response, "yes")
}
