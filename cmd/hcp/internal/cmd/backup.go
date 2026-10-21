package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// BackupRequest contains the operator-supplied values for one Velero backup.
// It deliberately does not depend on Cobra flag representations.
type BackupRequest struct {
	ClusterID   string
	Reason      string
	Labels      map[string]string
	Annotations map[string]string
}

func (r BackupRequest) Validate() error {
	if strings.TrimSpace(r.ClusterID) == "" {
		return fmt.Errorf("cluster ID is required")
	}
	if strings.TrimSpace(r.Reason) == "" {
		return fmt.Errorf("reason is required")
	}
	return nil
}

// backupIDPattern matches the backup name from velero's submission output, e.g.:
//
//	Backup request "2p49ok746l9e7o06v76t3ptp95k72heb-daily-20260319184212" submitted successfully.
var backupIDPattern = regexp.MustCompile(`Backup request "([^"]+)" submitted`)

// BackupCreator triggers a Velero backup for an HCP cluster.
type BackupCreator interface {
	CreateBackup(ctx context.Context, request BackupRequest) (BackupResult, error)
}

// BackupResult contains the Velero submission response.
type BackupResult struct {
	ID        string
	Namespace string
	RawOutput string
}

// defaultBackupCreatorConfig holds all configurable parameters for the backup creator,
// including operational settings and the cluster resolver. Use newDefaultBackupCreatorConfig to obtain an
// instance with defaults applied.
type defaultBackupCreatorConfig struct {
	ADPNamespace       string
	VeleroContainer    string
	VeleroLabelKey     string
	VeleroLabelValue   string
	ScheduleNameSuffix string
	// Resolver resolves a raw cluster identifier to canonical OCM IDs.
	// Defaults to an ocmClusterResolver constructed from the OCM connection
	// passed to NewDefaultBackupCreator. Override via WithResolver in tests.
	Resolver internalclient.ClusterResolver
}

// newDefaultBackupCreatorConfig returns a defaultBackupCreatorConfig populated with
// standard defaults. Call it once, then pass any overrides via DefaultBackupCreatorOption.
// Resolver is left nil here and configured by NewDefaultBackupCreator.
func newDefaultBackupCreatorConfig(opts ...DefaultBackupCreatorOption) defaultBackupCreatorConfig {
	cfg := defaultBackupCreatorConfig{
		ADPNamespace:       "openshift-adp",
		VeleroContainer:    "velero",
		VeleroLabelKey:     "app.kubernetes.io/name",
		VeleroLabelValue:   "velero",
		ScheduleNameSuffix: "-daily",
	}
	for _, o := range opts {
		o.ConfigureDefaultBackupCreator(&cfg)
	}
	return cfg
}

// DefaultBackupCreatorOption is implemented by any type that can configure a defaultBackupCreatorConfig.
// Concrete implementations live in options.go.
type DefaultBackupCreatorOption interface {
	ConfigureDefaultBackupCreator(*defaultBackupCreatorConfig)
}

// DefaultBackupCreator executes the backup workflow for an HCP cluster.
// All network I/O — OCM cluster resolution, backplane login, and Kubernetes
// operations — is deferred to Run so that construction is always cheap and
// side-effect free.
type DefaultBackupCreator struct {
	cfg       defaultBackupCreatorConfig
	ocmClient backupOCMClient
	resolver  internalclient.ClusterResolver
}

// NewDefaultBackupCreator constructs a DefaultBackupCreator that will use ocmClient
// for all OCM API calls made during Run. opts are applied after defaults and may
// override any field, including Resolver (useful in tests to avoid real
// network calls).
func NewDefaultBackupCreator(
	ocmClient backupOCMClient,
	opts ...DefaultBackupCreatorOption,
) *DefaultBackupCreator {
	cfg := newDefaultBackupCreatorConfig(opts...)

	resolver := cfg.Resolver
	if resolver == nil {
		resolver = ocmClient
	}

	return &DefaultBackupCreator{
		cfg:       cfg,
		ocmClient: ocmClient,
		resolver:  resolver,
	}
}

// CreateBackup executes the backup workflow: resolve cluster, validate schedule, find
// pod, trigger backup. All network I/O happens here — nothing is pre-fetched
// at construction time.
func (r *DefaultBackupCreator) CreateBackup(ctx context.Context, request BackupRequest) (BackupResult, error) {
	if err := request.Validate(); err != nil {
		return BackupResult{}, err
	}

	// Resolve the HCP cluster and its management cluster using the shared OCM
	// connection. This is the first network call in the workflow.
	clusterInfo, err := r.resolver.Resolve(ctx, request.ClusterID)
	if err != nil {
		return BackupResult{}, err
	}

	scheduleName := clusterInfo.HCPClusterID + r.cfg.ScheduleNameSuffix

	managementClient, err := r.ocmClient.ManagementCluster(clusterInfo.MgmtClusterID)
	if err != nil {
		return BackupResult{}, fmt.Errorf("connecting to management cluster %s: %w", clusterInfo.MgmtClusterID, err)
	}

	// Validate the Velero Schedule resource exists before attempting the backup.
	// This is done with the unprivileged client so that a missing schedule never
	// triggers an elevated login.
	if _, err := managementClient.GetVeleroSchedule(ctx, r.cfg.ADPNamespace, scheduleName); err != nil {
		if apierrors.IsNotFound(err) {
			return BackupResult{}, fmt.Errorf("velero schedule %q in namespace %q was not found: %w", scheduleName, r.cfg.ADPNamespace, err)
		}
		return BackupResult{}, err
	}

	managementClient, err = r.ocmClient.ElevatedManagementCluster(clusterInfo.MgmtClusterID, request.Reason)
	if err != nil {
		return BackupResult{}, fmt.Errorf("connecting to management cluster %s with elevation: %w", clusterInfo.MgmtClusterID, err)
	}

	output, err := managementClient.CreateVeleroBackup(ctx, internalclient.VeleroBackupRequest{
		Namespace: r.cfg.ADPNamespace, Schedule: scheduleName, Container: r.cfg.VeleroContainer,
		LabelKey: r.cfg.VeleroLabelKey, LabelValue: r.cfg.VeleroLabelValue,
		Labels: request.Labels, Annotations: request.Annotations,
	})
	if err != nil {
		return BackupResult{}, fmt.Errorf("triggering Velero backup from schedule %q: %w", scheduleName, err)
	}

	// Parse the backup ID from the velero submission output.
	// Example output:
	//   Creating backup from schedule, all other filters are ignored.
	//   Backup request "2p49ok746l9e7o06v76t3ptp95k72heb-daily-20260319184212" submitted successfully.
	//   Run `velero backup describe ...` or `velero backup logs ...` for more details.
	matches := backupIDPattern.FindStringSubmatch(output)
	if len(matches) < 2 {
		return BackupResult{Namespace: r.cfg.ADPNamespace, RawOutput: output}, nil
	}
	backupID := matches[1]

	return BackupResult{ID: backupID, Namespace: r.cfg.ADPNamespace, RawOutput: output}, nil
}

type backupOCMClient interface {
	internalclient.ClusterResolver
	ManagementCluster(clusterID string) (internalclient.ManagementClusterClient, error)
	ElevatedManagementCluster(clusterID, reason string) (internalclient.ManagementClusterClient, error)
}
