package cmd

import (
	"context"
	"fmt"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
)

// ReassignmentRequest identifies the HCP and its replacement management cluster.
type ReassignmentRequest struct {
	ClusterID     string
	ReplacementMC string
}

// ReassignmentManager prepares and applies an HCP management-cluster reassignment.
type ReassignmentManager interface {
	PrepareReassignment(context.Context, ReassignmentRequest) (ReassignmentPlan, error)
	Reassign(context.Context, ReassignmentPlan) (ReassignmentResult, error)
}

// ReassignmentPlan describes the reassignment ready for confirmation.
type ReassignmentPlan struct {
	HCPClusterID                   string
	SourceManagementCluster        string
	SourceManagementClusterID      string
	ReplacementManagementCluster   string
	ReplacementManagementClusterID string

	ocmClient reassignmentOCMClient
}

// ReassignmentResult reports whether the HCP was already assigned to the replacement.
type ReassignmentResult struct {
	AlreadyAssigned bool
}

// DefaultReassignmentManager is the production reassignment implementation.
type DefaultReassignmentManager struct{ ocmClient reassignmentOCMClient }

type reassignmentOCMClient interface {
	internalclient.ClusterResolver
	Cluster(identifier string) (*cmv1.Cluster, error)
	ManagementClusterAssignment(ctx context.Context, clusterID string) (string, error)
	UpdateManagementClusterAssignment(ctx context.Context, clusterID, managementCluster string) error
}

// NewDefaultReassignmentManager creates a reassignment manager backed by OCM.
func NewDefaultReassignmentManager(ocmClient reassignmentOCMClient) *DefaultReassignmentManager {
	return &DefaultReassignmentManager{ocmClient: ocmClient}
}

func (r *DefaultReassignmentManager) PrepareReassignment(ctx context.Context, request ReassignmentRequest) (ReassignmentPlan, error) {
	cluster, err := r.ocmClient.Resolve(ctx, request.ClusterID)
	if err != nil {
		return ReassignmentPlan{}, fmt.Errorf("resolving cluster: %w", err)
	}
	if cluster.MgmtClusterName == "" || cluster.MgmtClusterID == "" {
		return ReassignmentPlan{}, fmt.Errorf("source management cluster is incomplete")
	}
	source, err := r.ocmClient.Cluster(cluster.MgmtClusterID)
	if err != nil {
		return ReassignmentPlan{}, fmt.Errorf("resolving source management cluster %s: %w", cluster.MgmtClusterID, err)
	}
	if source.Region() == nil || source.Region().ID() == "" {
		return ReassignmentPlan{}, fmt.Errorf("source management cluster %s has no region", cluster.MgmtClusterName)
	}

	replacement, err := r.ocmClient.Cluster(request.ReplacementMC)
	if err != nil {
		return ReassignmentPlan{}, fmt.Errorf("resolving replacement management cluster %s: %w", request.ReplacementMC, err)
	}
	if replacement.Name() == "" || replacement.ID() == "" {
		return ReassignmentPlan{}, fmt.Errorf("replacement management cluster is incomplete")
	}
	if replacement.Region() == nil || replacement.Region().ID() == "" {
		return ReassignmentPlan{}, fmt.Errorf("replacement management cluster %s has no region", replacement.Name())
	}
	if replacement.Region().ID() != source.Region().ID() {
		return ReassignmentPlan{}, fmt.Errorf("replacement management cluster %s is in region %s, expected source management cluster region %s", replacement.Name(), replacement.Region().ID(), source.Region().ID())
	}

	return ReassignmentPlan{
		HCPClusterID:                   cluster.HCPClusterID,
		SourceManagementCluster:        cluster.MgmtClusterName,
		SourceManagementClusterID:      cluster.MgmtClusterID,
		ReplacementManagementCluster:   replacement.Name(),
		ReplacementManagementClusterID: replacement.ID(),
		ocmClient:                      r.ocmClient,
	}, nil
}

func (r *DefaultReassignmentManager) Reassign(ctx context.Context, plan ReassignmentPlan) (ReassignmentResult, error) {
	current, err := plan.ocmClient.ManagementClusterAssignment(ctx, plan.HCPClusterID)
	if err != nil {
		return ReassignmentResult{}, fmt.Errorf("reading current management cluster assignment: %w", err)
	}
	if current == plan.ReplacementManagementCluster {
		return ReassignmentResult{AlreadyAssigned: true}, nil
	}
	if current != plan.SourceManagementCluster {
		return ReassignmentResult{}, fmt.Errorf("management cluster assignment changed from %s to %s after preview", plan.SourceManagementCluster, current)
	}

	if err := plan.ocmClient.UpdateManagementClusterAssignment(ctx, plan.HCPClusterID, plan.ReplacementManagementCluster); err != nil {
		return ReassignmentResult{}, err
	}

	current, err = plan.ocmClient.ManagementClusterAssignment(ctx, plan.HCPClusterID)
	if err == nil && current == plan.ReplacementManagementCluster {
		return ReassignmentResult{}, nil
	}
	if err != nil {
		err = fmt.Errorf("reading management cluster assignment after update: %w", err)
	} else {
		err = fmt.Errorf("management cluster assignment is %s after update, expected %s", current, plan.ReplacementManagementCluster)
	}
	return ReassignmentResult{}, err
}
