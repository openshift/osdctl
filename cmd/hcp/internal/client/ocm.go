package client

import (
	"context"
	"fmt"

	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ── Cluster resolution ─────────────────────────────────────────────────────

// ClusterInfo holds cluster details resolved from OCM by a ClusterResolver.
type ClusterInfo struct {
	HCPClusterID     string
	HCPClusterRegion string
	DomainPrefix     string
	BaseDomain       string
	MgmtClusterID    string
	MgmtClusterName  string
}

// ServiceClusterInfo identifies the Service Cluster that hosts an HCP's
// ManifestWorks.
type ServiceClusterInfo struct {
	ID   string
	Name string
}

// OCMClient provides HCP operations backed by a shared OCM connection.
// The caller owns the connection lifecycle.
type OCMClient struct {
	conn   *ocmsdk.Connection
	logger *logrus.Logger
}

func NewOCMClient(conn *ocmsdk.Connection, logger *logrus.Logger) *OCMClient {
	return &OCMClient{conn: conn, logger: logger}
}

// ClusterResolver resolves an HCP cluster identifier to its canonical OCM IDs,
// including the management cluster.
type ClusterResolver interface {
	Resolve(ctx context.Context, clusterIdentifier string) (ClusterInfo, error)
}

func (c *OCMClient) ResolveCluster(ctx context.Context, clusterIdentifier string) (ClusterInfo, error) {
	hcpCluster, err := utils.GetClusterAnyStatusWithContext(ctx, c.conn, clusterIdentifier)
	if err != nil {
		return ClusterInfo{}, fmt.Errorf("retrieving HCP cluster %q: %w", clusterIdentifier, err)
	}
	clusterID := hcpCluster.ID()

	mc, err := utils.GetManagementClusterWithConn(ctx, clusterID, c.conn)
	if err != nil {
		return ClusterInfo{}, fmt.Errorf("getting management cluster for %s: %w", clusterID, err)
	}
	region := ""
	if hcpCluster.Region() != nil {
		region = hcpCluster.Region().ID()
	}
	baseDomain := ""
	if hcpCluster.DNS() != nil {
		baseDomain = hcpCluster.DNS().BaseDomain()
	}

	return ClusterInfo{
		HCPClusterID:     clusterID,
		HCPClusterRegion: region,
		DomainPrefix:     hcpCluster.DomainPrefix(),
		BaseDomain:       baseDomain,
		MgmtClusterID:    mc.ID(),
		MgmtClusterName:  mc.Name(),
	}, nil
}

func (c *OCMClient) Resolve(ctx context.Context, clusterIdentifier string) (ClusterInfo, error) {
	return c.ResolveCluster(ctx, clusterIdentifier)
}

func (c *OCMClient) ResolveServiceCluster(ctx context.Context, clusterID string) (ServiceClusterInfo, error) {
	hypershiftResp, err := c.conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Hypershift().Get().SendContext(ctx)
	if err != nil {
		return ServiceClusterInfo{}, fmt.Errorf("getting hypershift info for cluster %s: %w", clusterID, err)
	}
	managementCluster, ok := hypershiftResp.Body().GetManagementCluster()
	if !ok || managementCluster == "" {
		return ServiceClusterInfo{}, fmt.Errorf("no management cluster found for %s", clusterID)
	}

	managementClusters, err := c.conn.OSDFleetMgmt().V1().ManagementClusters().List().
		Parameter("search", fmt.Sprintf("name='%s'", managementCluster)).SendContext(ctx)
	if err != nil {
		return ServiceClusterInfo{}, fmt.Errorf("getting fleet manager information for management cluster %s: %w", managementCluster, err)
	}
	if managementClusters.Items().Empty() {
		return ServiceClusterInfo{}, fmt.Errorf("no fleet manager information found for management cluster %s", managementCluster)
	}
	parent := managementClusters.Items().Get(0).Parent()
	if parent == nil || parent.Kind() != "ServiceCluster" || parent.Name() == "" {
		return ServiceClusterInfo{}, fmt.Errorf("no service cluster found for management cluster %s", managementCluster)
	}
	serviceCluster, err := utils.GetClusterAnyStatusWithContext(ctx, c.conn, parent.Name())
	if err != nil {
		return ServiceClusterInfo{}, fmt.Errorf("getting service cluster %s: %w", parent.Name(), err)
	}
	return ServiceClusterInfo{ID: serviceCluster.ID(), Name: serviceCluster.Name()}, nil
}

func (c *OCMClient) Cluster(identifier string) (*cmv1.Cluster, error) {
	return utils.GetClusterAnyStatus(c.conn, identifier)
}

// ManagementClusterAssignment returns the management cluster currently assigned
// to the HCP in OCM.
func (c *OCMClient) ManagementClusterAssignment(ctx context.Context, clusterID string) (string, error) {
	response, err := c.conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Hypershift().Get().SendContext(ctx)
	if err != nil {
		return "", fmt.Errorf("getting hypershift info for cluster %s: %w", clusterID, err)
	}
	if response == nil || response.Body() == nil {
		return "", fmt.Errorf("no hypershift info found for %s", clusterID)
	}
	managementCluster, ok := response.Body().GetManagementCluster()
	if !ok || managementCluster == "" {
		return "", fmt.Errorf("no management cluster found for %s", clusterID)
	}
	return managementCluster, nil
}

// UpdateManagementClusterAssignment patches only the HCP management cluster.
func (c *OCMClient) UpdateManagementClusterAssignment(ctx context.Context, clusterID, managementCluster string) error {
	config, err := cmv1.NewHypershiftConfig().ManagementCluster(managementCluster).Build()
	if err != nil {
		return fmt.Errorf("building hypershift update for cluster %s: %w", clusterID, err)
	}
	_, err = c.conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Hypershift().Update().Body(config).SendContext(ctx)
	if err != nil {
		return fmt.Errorf("updating management cluster for %s: %w", clusterID, err)
	}
	return nil
}

func (c *OCMClient) BackupConfig(ctx context.Context, clusterID, managementCluster string) (*cmv1.AWSBackupConfig, error) {
	if managementCluster == "" {
		return nil, fmt.Errorf("no management cluster found for %s", clusterID)
	}

	response, err := c.conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).ProvisionShard().Get().SendContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting provision shard for cluster %s: %w", clusterID, err)
	}
	if response == nil || response.Body() == nil || response.Body().HypershiftConfig() == nil || response.Body().HypershiftConfig().AWSShard() == nil {
		return nil, fmt.Errorf("no AWS backup configuration found for cluster %s", clusterID)
	}

	return selectBackupConfig(response.Body().HypershiftConfig().AWSShard().BackupConfigs(), managementCluster)
}

func (c *OCMClient) Environment() string {
	return utils.GetCurrentOCMEnv(c.conn)
}

func selectBackupConfig(configs []*cmv1.AWSBackupConfig, managementCluster string) (*cmv1.AWSBackupConfig, error) {
	var matched *cmv1.AWSBackupConfig
	for _, config := range configs {
		if config == nil || config.ManagementCluster() != managementCluster {
			continue
		}
		if matched != nil {
			return nil, fmt.Errorf("multiple backup configurations found for management cluster %s", managementCluster)
		}
		matched = config
	}
	if matched == nil {
		return nil, fmt.Errorf("no backup configuration found for management cluster %s", managementCluster)
	}
	if matched.S3Bucket() == "" || matched.AccountId() == "" {
		return nil, fmt.Errorf("backup configuration for management cluster %s is incomplete", managementCluster)
	}
	return matched, nil
}

// ManagementClusterClient manages Velero resources for one management cluster.
// Schedule checks remain unprivileged; creating a backup requires elevation.
type ManagementClusterClient interface {
	GetVeleroSchedule(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error)
	CreateVeleroBackup(ctx context.Context, request VeleroBackupRequest) (string, error)
	RestoreVeleroBackup(ctx context.Context, request VeleroRestoreRequest) (VeleroRestoreResult, error)
}

// ServiceClusterClient manages ManifestWorks for one service cluster.
type ServiceClusterClient interface {
	GetManifestWork(ctx context.Context, namespace, name string) (*workv1.ManifestWork, error)
	ListManifestWorks(ctx context.Context, namespace string, options ...ManifestWorkListOption) ([]workv1.ManifestWork, error)
	CreateManifestWork(ctx context.Context, work *workv1.ManifestWork) error
	UpdateManifestWork(ctx context.Context, work *workv1.ManifestWork) error
}

type clusterConnection struct {
	client    client.Client
	restCfg   *rest.Config
	clientset kubernetes.Interface
}

func (c *OCMClient) newClusterConnection(clusterID string, elevationReasons ...string) (*clusterConnection, error) {
	kubeCli, restCfg, clientset, err := common.GetKubeConfigAndClientWithConn(clusterID, c.conn, elevationReasons...)
	if err != nil {
		return nil, err
	}
	if err := workv1.AddToScheme(kubeCli.Scheme()); err != nil {
		return nil, fmt.Errorf("adding ManifestWork scheme: %w", err)
	}
	return &clusterConnection{client: kubeCli, restCfg: restCfg, clientset: clientset}, nil
}

func (c *OCMClient) ManagementCluster(clusterID string) (ManagementClusterClient, error) {
	c.logger.Infof("Logging into cluster %s...", clusterID)
	return c.newManagementClusterClient(clusterID)
}

func (c *OCMClient) ElevatedManagementCluster(clusterID, reason string) (ManagementClusterClient, error) {
	c.logger.Infof("Logging into cluster %s (elevated)...", clusterID)
	return c.newManagementClusterClient(clusterID, reason)
}

func (c *OCMClient) ServiceCluster(clusterID, reason string) (ServiceClusterClient, error) {
	c.logger.Infof("Logging into cluster %s (elevated)...", clusterID)
	connection, err := c.newClusterConnection(clusterID, reason)
	if err != nil {
		return nil, err
	}
	return &serviceClusterClient{client: connection.client}, nil
}

func (c *OCMClient) newManagementClusterClient(clusterID string, elevationReasons ...string) (*managementClusterClient, error) {
	connection, err := c.newClusterConnection(clusterID, elevationReasons...)
	if err != nil {
		return nil, err
	}
	return &managementClusterClient{client: connection.client, restCfg: connection.restCfg, clientset: connection.clientset}, nil
}
