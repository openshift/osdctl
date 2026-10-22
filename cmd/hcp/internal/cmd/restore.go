package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
)

// RestoreRequest contains the operator-supplied values for one Velero restore.
type RestoreRequest struct {
	ClusterID  string
	TargetMC   string
	BackupName string
	Reason     string
	Timeout    time.Duration
}

func (r RestoreRequest) Validate() error {
	if strings.TrimSpace(r.ClusterID) == "" {
		return fmt.Errorf("cluster ID is required")
	}
	if strings.TrimSpace(r.BackupName) == "" {
		return fmt.Errorf("backup name is required")
	}
	if strings.TrimSpace(r.Reason) == "" {
		return fmt.Errorf("reason is required")
	}
	if r.Timeout <= 0 {
		return fmt.Errorf("timeout must be greater than zero")
	}
	return nil
}

// RestoreResult reports the completed managed Velero restore.
type RestoreResult struct {
	Name    string
	Created bool
}

// RestoreCreator creates and waits for a Velero restore.
type RestoreCreator interface {
	CreateRestore(context.Context, RestoreRequest) (RestoreResult, error)
}

// DefaultRestoreCreator is the production RestoreCreator implementation.
type DefaultRestoreCreator struct{ ocmClient restoreOCMClient }

func NewDefaultRestoreCreator(ocmClient restoreOCMClient) *DefaultRestoreCreator {
	return &DefaultRestoreCreator{ocmClient: ocmClient}
}

func (c *DefaultRestoreCreator) CreateRestore(ctx context.Context, request RestoreRequest) (RestoreResult, error) {
	if err := request.Validate(); err != nil {
		return RestoreResult{}, err
	}
	cluster, err := c.ocmClient.Resolve(ctx, request.ClusterID)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("resolving cluster: %w", err)
	}
	targetMCID := cluster.MgmtClusterID
	verifyBackup := true
	if request.TargetMC != "" {
		target, err := c.ocmClient.Cluster(request.TargetMC)
		if err != nil {
			return RestoreResult{}, fmt.Errorf("resolving target management cluster %s: %w", request.TargetMC, err)
		}
		if cluster.HCPClusterRegion == "" {
			return RestoreResult{}, fmt.Errorf("HCP cluster %s has no region", cluster.HCPClusterID)
		}
		if target.Region() == nil || target.Region().ID() == "" {
			return RestoreResult{}, fmt.Errorf("target management cluster %s has no region", request.TargetMC)
		}
		if target.Region().ID() != cluster.HCPClusterRegion {
			return RestoreResult{}, fmt.Errorf("target management cluster %s is in region %s, expected HCP region %s", request.TargetMC, target.Region().ID(), cluster.HCPClusterRegion)
		}
		targetMCID = target.ID()
		verifyBackup = false
	}
	if targetMCID == "" {
		return RestoreResult{}, fmt.Errorf("target management cluster has no ID")
	}

	managementClient, err := c.ocmClient.ElevatedManagementCluster(targetMCID, request.Reason)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("connecting to target management cluster: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	result, err := managementClient.RestoreVeleroBackup(waitCtx, internalclient.VeleroRestoreRequest{ClusterID: cluster.HCPClusterID, BackupName: request.BackupName, VerifyBackup: verifyBackup})
	if err != nil {
		return RestoreResult{}, err
	}
	return RestoreResult{Name: result.Name, Created: result.Created}, nil
}

type restoreOCMClient interface {
	internalclient.ClusterResolver
	Cluster(identifier string) (*cmv1.Cluster, error)
	ElevatedManagementCluster(clusterID, reason string) (internalclient.ManagementClusterClient, error)
}
