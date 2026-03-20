package backup

import (
	"context"
	"fmt"

	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
)

// ── Cluster resolution ─────────────────────────────────────────────────────

// ClusterInfo holds the canonical OCM internal IDs for the HCP cluster
// and its management cluster, as resolved by a ClusterResolver.
type ClusterInfo struct {
	HCPClusterID  string
	MgmtClusterID string
}

// ClusterResolver resolves an HCP cluster identifier to its canonical OCM IDs,
// including the management cluster. The OCM connection used for resolution is
// provided at construction time; the caller is responsible for its lifetime.
type ClusterResolver interface {
	Resolve(ctx context.Context, clusterIdentifier string) (ClusterInfo, error)
}

// ocmClusterResolver is the production ClusterResolver. It reuses the OCM
// connection provided at construction time so no second connection is opened.
type ocmClusterResolver struct {
	ocmConn *ocmsdk.Connection
	logger  *logrus.Logger
}

// Resolve looks up the HCP cluster by clusterIdentifier and then fetches the
// management cluster via the Hypershift API, all using the shared OCM connection.
func (r *ocmClusterResolver) Resolve(ctx context.Context, clusterIdentifier string) (ClusterInfo, error) {
	hcpCluster, err := utils.GetClusterAnyStatus(r.ocmConn, clusterIdentifier)
	if err != nil {
		return ClusterInfo{}, fmt.Errorf("retrieving HCP cluster %q: %w", clusterIdentifier, err)
	}
	clusterID := hcpCluster.ID()

	r.logger.Infof("Resolving management cluster for HCP cluster %s...", clusterID)

	hypershiftResp, err := r.ocmConn.ClustersMgmt().V1().Clusters().
		Cluster(clusterID).Hypershift().Get().SendContext(ctx)
	if err != nil {
		return ClusterInfo{}, fmt.Errorf("getting hypershift info for cluster %s: %w", clusterID, err)
	}
	mgmtClusterName, ok := hypershiftResp.Body().GetManagementCluster()
	if !ok {
		return ClusterInfo{}, fmt.Errorf("no management cluster found for %s", clusterID)
	}
	mc, err := utils.GetClusterAnyStatus(r.ocmConn, mgmtClusterName)
	if err != nil {
		return ClusterInfo{}, fmt.Errorf("getting management cluster %s: %w", mgmtClusterName, err)
	}

	return ClusterInfo{
		HCPClusterID:  clusterID,
		MgmtClusterID: mc.ID(),
	}, nil
}

// ── KubeClient building ────────────────────────────────────────────────────

// buildConfig holds the options accumulated by a single Build call.
type buildConfig struct {
	clusterID       string
	elevated        bool
	elevationReason string
}

// BuildOption configures a single call to KubeClientBuilder.Build.
type BuildOption interface {
	configureBuild(*buildConfig)
}

// WithClusterID sets the target cluster for a Build call.
type WithClusterID struct{ ClusterID string }

func (w WithClusterID) configureBuild(c *buildConfig) {
	c.clusterID = w.ClusterID
}

// WithElevation requests a backplane-cluster-admin session.
// Reason is the justification string passed to backplane and is required.
type WithElevation struct{ Reason string }

func (w WithElevation) configureBuild(c *buildConfig) {
	c.elevated = true
	c.elevationReason = w.Reason
}

// KubeClientBuilder creates KubeClient instances. Callers express their
// requirements via BuildOption values; the builder interprets them and performs
// whatever login or elevation is necessary.
type KubeClientBuilder interface {
	Build(ctx context.Context, opts ...BuildOption) (KubeClient, error)
}

// backplaneClientBuilder is the production KubeClientBuilder. It logs into the
// cluster specified by WithClusterID via backplane, reusing the caller's OCM
// connection to avoid opening a second connection. Pass WithElevation to obtain
// a backplane-cluster-admin session.
type backplaneClientBuilder struct {
	ocmConn *ocmsdk.Connection
	logger  *logrus.Logger
}

// Build creates a KubeClient for the cluster identified by WithClusterID. If
// WithElevation is also included the session is elevated to
// backplane-cluster-admin.
func (b *backplaneClientBuilder) Build(ctx context.Context, opts ...BuildOption) (KubeClient, error) {
	cfg := &buildConfig{}
	for _, o := range opts {
		o.configureBuild(cfg)
	}

	if cfg.elevated {
		b.logger.Infof("Logging into cluster %s (elevated)...", cfg.clusterID)
		return newKubeClientForCluster(b.ocmConn, cfg.clusterID, cfg.elevationReason)
	}

	b.logger.Infof("Logging into cluster %s...", cfg.clusterID)
	return newKubeClientForCluster(b.ocmConn, cfg.clusterID)
}
