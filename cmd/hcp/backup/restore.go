package backup

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	veleroAPIGroup           = "velero.io"
	restoreManagedLabel      = "osdctl.openshift.io/managed"
	restoreManagedValue      = "hcp-backup-restore"
	restoreBackupAnnotation  = "osdctl.openshift.io/backup-name"
	restoreClusterAnnotation = "osdctl.openshift.io/cluster-id"
	restoreAttemptAnnotation = "osdctl.openshift.io/attempt"
)

type veleroResources struct {
	groupVersion schema.GroupVersion
	backupKind   string
	restoreKind  string
	bslKind      string
}

type restoreFlags struct {
	clusterID  string
	targetMC   string
	backupName string
	reason     string
	timeout    time.Duration
}

type restoreRunner struct {
	resolver     ClusterResolver
	getCluster   func(string) (*cmv1.Cluster, error)
	builder      KubeClientBuilder
	out          io.Writer
	pollInterval time.Duration
}

func newCmdRestore() *cobra.Command {
	flags := &restoreFlags{}
	cmd := &cobra.Command{
		Use:   "restore --cluster-id <cluster-id> --backup-name <backup-name> --reason <reason> [--target-mc <management-cluster>] [--timeout <duration>]",
		Short: "Create and monitor a Velero restore for an HCP backup",
		Long:  "Create and monitor a Velero Restore in the target management cluster. The target defaults to the management cluster currently bound to the HCP; an explicit target must be in the HCP's region. Repeated invocations safely resume an active restore or report an already completed restore; failed attempts receive a new retry Restore.",
		Example: "  osdctl hcp backup restore --cluster-id ${CLUSTER_ID} --backup-name ${BACKUP_NAME} --reason ${REASON}\n" +
			"  osdctl hcp backup restore --cluster-id ${CLUSTER_ID} --target-mc ${TARGET_MC} --backup-name ${BACKUP_NAME} --reason ${REASON} --timeout 30m",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := logrus.New()
			logger.SetOutput(cmd.ErrOrStderr())
			ocmConn, err := utils.CreateConnection()
			if err != nil {
				return fmt.Errorf("creating OCM connection: %w", err)
			}
			defer ocmConn.Close()

			runner := newRestoreRunner(ocmConn, logger, cmd.OutOrStdout())
			return runner.run(cmd.Context(), *flags)
		},
	}
	cmd.Flags().StringVarP(&flags.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().StringVar(&flags.targetMC, "target-mc", "", "Internal ID, name, or external ID of the management cluster that will run the restore")
	cmd.Flags().StringVar(&flags.backupName, "backup-name", "", "Name of the completed Velero backup to restore")
	cmd.Flags().StringVar(&flags.reason, "reason", "", "Reason for elevated access")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 30*time.Minute, "Maximum time to wait for the restore to complete")
	for _, flag := range []string{"cluster-id", "backup-name", "reason"} {
		_ = cmd.MarkFlagRequired(flag)
	}
	return cmd
}

func newRestoreRunner(ocmConn *ocmsdk.Connection, logger *logrus.Logger, out io.Writer) *restoreRunner {
	return &restoreRunner{
		resolver: &ocmClusterResolver{ocmConn: ocmConn, logger: logger},
		getCluster: func(identifier string) (*cmv1.Cluster, error) {
			return utils.GetClusterAnyStatus(ocmConn, identifier)
		},
		builder:      &backplaneClientBuilder{ocmConn: ocmConn, logger: logger},
		out:          out,
		pollInterval: 5 * time.Second,
	}
}

func (r *restoreRunner) run(ctx context.Context, flags restoreFlags) error {
	if flags.timeout <= 0 {
		return fmt.Errorf("timeout must be greater than zero")
	}
	cluster, err := r.resolver.Resolve(ctx, flags.clusterID)
	if err != nil {
		return fmt.Errorf("resolving cluster: %w", err)
	}
	targetMCID := cluster.MgmtClusterID
	if flags.targetMC != "" {
		target, err := r.getCluster(flags.targetMC)
		if err != nil {
			return fmt.Errorf("resolving target management cluster %s: %w", flags.targetMC, err)
		}
		if cluster.HCPClusterRegion == "" {
			return fmt.Errorf("HCP cluster %s has no region", cluster.HCPClusterID)
		}
		if target.Region() == nil || target.Region().ID() == "" {
			return fmt.Errorf("target management cluster %s has no region", flags.targetMC)
		}
		if target.Region().ID() != cluster.HCPClusterRegion {
			return fmt.Errorf("target management cluster %s is in region %s, expected HCP region %s", flags.targetMC, target.Region().ID(), cluster.HCPClusterRegion)
		}
		targetMCID = target.ID()
	}
	if targetMCID == "" {
		return fmt.Errorf("target management cluster has no ID")
	}

	client, err := r.builder.Build(ctx, WithClusterID{ClusterID: targetMCID}, WithElevation{Reason: flags.reason})
	if err != nil {
		return fmt.Errorf("building target management cluster client: %w", err)
	}
	resources, err := discoverVeleroResources(client)
	if err != nil {
		return err
	}
	if err := verifyBackupStorageLocation(ctx, client, resources); err != nil {
		return err
	}
	if flags.targetMC == "" {
		if err := verifyBackup(ctx, client, resources, flags.backupName); err != nil {
			return err
		}
	}

	restore, created, err := r.getOrCreateRestore(ctx, client, resources, cluster.HCPClusterID, flags.backupName)
	if err != nil {
		return err
	}
	if created {
		fmt.Fprintf(r.out, "Created Velero Restore %s for backup %s.\n", restore.GetName(), flags.backupName)
	} else {
		fmt.Fprintf(r.out, "Monitoring existing Velero Restore %s for backup %s.\n", restore.GetName(), flags.backupName)
	}

	waitCtx, cancel := context.WithTimeout(ctx, flags.timeout)
	defer cancel()
	if err := r.waitForRestore(waitCtx, client, resources, restore.GetName(), flags.backupName); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Velero Restore %s completed.\n", restore.GetName())
	return nil
}

func discoverVeleroResources(kubeClient KubeClient) (veleroResources, error) {
	groups, err := kubeClient.ServerGroups()
	if err != nil {
		return veleroResources{}, fmt.Errorf("discovering API groups: %w", err)
	}
	for _, group := range groups.Groups {
		if group.Name != veleroAPIGroup {
			continue
		}
		groupVersion, err := schema.ParseGroupVersion(group.PreferredVersion.GroupVersion)
		if err != nil || groupVersion.Group != veleroAPIGroup || groupVersion.Version == "" {
			return veleroResources{}, fmt.Errorf("invalid preferred Velero API version %q", group.PreferredVersion.GroupVersion)
		}
		resourceList, err := kubeClient.ServerResourcesForGroupVersion(groupVersion.String())
		if err != nil {
			return veleroResources{}, fmt.Errorf("discovering Velero resources for %s: %w", groupVersion, err)
		}
		resources := veleroResources{groupVersion: groupVersion}
		for _, resource := range resourceList.APIResources {
			switch resource.Name {
			case "backups":
				resources.backupKind = resource.Kind
			case "restores":
				resources.restoreKind = resource.Kind
			case "backupstoragelocations":
				resources.bslKind = resource.Kind
			}
		}
		if resources.backupKind == "" || resources.restoreKind == "" || resources.bslKind == "" {
			return veleroResources{}, fmt.Errorf("velero API %s is missing required resources: backups=%t restores=%t backupstoragelocations=%t", groupVersion, resources.backupKind != "", resources.restoreKind != "", resources.bslKind != "")
		}
		return resources, nil
	}
	return veleroResources{}, fmt.Errorf("velero API group %s is not available", veleroAPIGroup)
}

func verifyBackupStorageLocation(ctx context.Context, kubeClient KubeClient, resources veleroResources) error {
	locations := veleroList(resources, resources.bslKind)
	if err := kubeClient.List(ctx, locations, client.InNamespace(veleroNamespace)); err != nil {
		return fmt.Errorf("listing BackupStorageLocations: %w", err)
	}
	phases := make([]string, 0, len(locations.Items))
	for i := range locations.Items {
		phase, _, _ := unstructured.NestedString(locations.Items[i].Object, "status", "phase")
		if phase == "Available" {
			return nil
		}
		phases = append(phases, fmt.Sprintf("%s=%s", locations.Items[i].GetName(), phase))
	}
	if len(phases) == 0 {
		return fmt.Errorf("no BackupStorageLocations found in namespace %s", veleroNamespace)
	}
	return fmt.Errorf("no available BackupStorageLocation in namespace %s (%s)", veleroNamespace, strings.Join(phases, ", "))
}

func verifyBackup(ctx context.Context, kubeClient KubeClient, resources veleroResources, backupName string) error {
	backup := veleroObject(resources, resources.backupKind, backupName)
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: veleroNamespace, Name: backupName}, backup); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("velero Backup %s was not found in namespace %s", backupName, veleroNamespace)
		}
		return fmt.Errorf("getting Velero Backup %s: %w", backupName, err)
	}
	phase, _, _ := unstructured.NestedString(backup.Object, "status", "phase")
	if phase != "Completed" {
		return fmt.Errorf("velero Backup %s is %q, expected Completed", backupName, phase)
	}
	return nil
}

func (r *restoreRunner) getOrCreateRestore(ctx context.Context, kubeClient KubeClient, resources veleroResources, clusterID, backupName string) (*unstructured.Unstructured, bool, error) {
	restores := veleroList(resources, resources.restoreKind)
	if err := kubeClient.List(ctx, restores, client.InNamespace(veleroNamespace), client.MatchingLabels{restoreManagedLabel: restoreManagedValue}); err != nil {
		return nil, false, fmt.Errorf("listing Velero Restores: %w", err)
	}

	maxAttempt := 0
	var active *unstructured.Unstructured
	for i := range restores.Items {
		restore := &restores.Items[i]
		if restore.GetAnnotations()[restoreBackupAnnotation] != backupName ||
			restore.GetAnnotations()[restoreClusterAnnotation] != clusterID {
			continue
		}
		attempt, _ := strconv.Atoi(restore.GetAnnotations()[restoreAttemptAnnotation])
		if attempt > maxAttempt {
			maxAttempt = attempt
		}
		phase, _, _ := unstructured.NestedString(restore.Object, "status", "phase")
		if phase == "Completed" {
			return restore.DeepCopy(), false, nil
		}
		if !isTerminalRestorePhase(phase) && active == nil {
			active = restore.DeepCopy()
		}
	}
	if active != nil {
		return active, false, nil
	}

	attempt := maxAttempt + 1
	restore := newVeleroRestore(resources, clusterID, backupName, attempt)
	if err := kubeClient.Create(ctx, restore); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, false, fmt.Errorf("creating Velero Restore %s for backup %s: %w", restore.GetName(), backupName, err)
		}
		if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: veleroNamespace, Name: restore.GetName()}, restore); err != nil {
			return nil, false, fmt.Errorf("getting concurrently created Velero Restore %s: %w", restore.GetName(), err)
		}
		if restore.GetLabels()[restoreManagedLabel] != restoreManagedValue ||
			restore.GetAnnotations()[restoreBackupAnnotation] != backupName ||
			restore.GetAnnotations()[restoreClusterAnnotation] != clusterID {
			return nil, false, fmt.Errorf("velero Restore %s already exists but is not managed for cluster %s and backup %s", restore.GetName(), clusterID, backupName)
		}
		return restore, false, nil
	}
	return restore, true, nil
}

func (r *restoreRunner) waitForRestore(ctx context.Context, kubeClient KubeClient, resources veleroResources, restoreName, backupName string) error {
	interval := r.pollInterval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		restore := veleroObject(resources, resources.restoreKind, restoreName)
		err := kubeClient.Get(ctx, types.NamespacedName{Namespace: veleroNamespace, Name: restoreName}, restore)
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("waiting for Velero Restore %s for backup %s: %w", restoreName, backupName, ctx.Err())
			}
			return fmt.Errorf("getting Velero Restore %s for backup %s: %w", restoreName, backupName, err)
		}
		phase, _, _ := unstructured.NestedString(restore.Object, "status", "phase")
		switch phase {
		case "Completed":
			return nil
		case "Failed", "FailedValidation", "PartiallyFailed":
			return fmt.Errorf("velero Restore %s for backup %s reached %s: %s", restoreName, backupName, phase, restoreDiagnostics(restore))
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for Velero Restore %s for backup %s (last phase %q): %w", restoreName, backupName, phase, ctx.Err())
		case <-ticker.C:
		}
	}
}

func newVeleroRestore(resources veleroResources, clusterID, backupName string, attempt int) *unstructured.Unstructured {
	name := restoreName(clusterID, backupName, attempt)
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": resources.groupVersion.String(),
		"kind":       resources.restoreKind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": veleroNamespace,
			"labels":    map[string]interface{}{restoreManagedLabel: restoreManagedValue},
			"annotations": map[string]interface{}{
				restoreBackupAnnotation:  backupName,
				restoreClusterAnnotation: clusterID,
				restoreAttemptAnnotation: strconv.Itoa(attempt),
			},
		},
		"spec": map[string]interface{}{"backupName": backupName},
	}}
}

func restoreName(clusterID, backupName string, attempt int) string {
	sum := sha256.Sum256([]byte(clusterID + "\x00" + backupName))
	return fmt.Sprintf("osdctl-restore-%x-%d", sum[:6], attempt)
}

func veleroObject(resources veleroResources, kind, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: resources.groupVersion.Group, Version: resources.groupVersion.Version, Kind: kind})
	obj.SetNamespace(veleroNamespace)
	obj.SetName(name)
	return obj
}

func veleroList(resources veleroResources, kind string) *unstructured.UnstructuredList {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{Group: resources.groupVersion.Group, Version: resources.groupVersion.Version, Kind: kind + "List"})
	return list
}

func isTerminalRestorePhase(phase string) bool {
	return phase == "Completed" || phase == "Failed" || phase == "FailedValidation" || phase == "PartiallyFailed"
}

func restoreDiagnostics(restore *unstructured.Unstructured) string {
	fields := []string{"failureReason", "validationErrors", "warnings", "errors"}
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		if value, ok, _ := unstructured.NestedFieldCopy(restore.Object, "status", field); ok && value != nil {
			values = append(values, fmt.Sprintf("%s=%v", field, value))
		}
	}
	if len(values) == 0 {
		return "no status diagnostics"
	}
	return strings.Join(values, ", ")
}
