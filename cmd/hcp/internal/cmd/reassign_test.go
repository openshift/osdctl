package cmd

import (
	"context"
	"errors"
	"fmt"
	"testing"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type assignmentResponse struct {
	assignment string
	err        error
}

type testReassignmentOCMClient struct {
	cluster     ClusterInfo
	clusters    map[string]*cmv1.Cluster
	assignments []assignmentResponse
	updates     []string
	updateErrs  []error
}

func (c *testReassignmentOCMClient) Resolve(_ context.Context, _ string) (ClusterInfo, error) {
	return c.cluster, nil
}

func (c *testReassignmentOCMClient) Cluster(identifier string) (*cmv1.Cluster, error) {
	cluster, ok := c.clusters[identifier]
	if !ok {
		return nil, fmt.Errorf("unexpected cluster %s", identifier)
	}
	return cluster, nil
}

func (c *testReassignmentOCMClient) ManagementClusterAssignment(_ context.Context, _ string) (string, error) {
	if len(c.assignments) == 0 {
		return "", errors.New("unexpected assignment read")
	}
	response := c.assignments[0]
	c.assignments = c.assignments[1:]
	return response.assignment, response.err
}

func (c *testReassignmentOCMClient) UpdateManagementClusterAssignment(_ context.Context, _ string, assignment string) error {
	c.updates = append(c.updates, assignment)
	if len(c.updateErrs) == 0 {
		return nil
	}
	err := c.updateErrs[0]
	c.updateErrs = c.updateErrs[1:]
	return err
}

func TestPrepareReassignment(t *testing.T) {
	for _, tt := range []struct {
		name              string
		sourceRegion      string
		replacementRegion string
		wantErr           string
	}{
		{name: "matching regions", sourceRegion: "us-east-1", replacementRegion: "us-east-1"},
		{name: "different regions", sourceRegion: "us-east-1", replacementRegion: "us-west-2", wantErr: "expected source management cluster region us-east-1"},
		{name: "missing source region", replacementRegion: "us-east-1", wantErr: "source management cluster source-mc has no region"},
		{name: "missing replacement region", sourceRegion: "us-east-1", wantErr: "replacement management cluster replacement-mc has no region"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sourceBuilder := cmv1.NewCluster().ID("source-id").Name("source-mc")
			if tt.sourceRegion != "" {
				sourceBuilder.Region(cmv1.NewCloudRegion().ID(tt.sourceRegion))
			}
			source, err := sourceBuilder.Build()
			require.NoError(t, err)

			replacementBuilder := cmv1.NewCluster().ID("replacement-id").Name("replacement-mc")
			if tt.replacementRegion != "" {
				replacementBuilder.Region(cmv1.NewCloudRegion().ID(tt.replacementRegion))
			}
			replacement, err := replacementBuilder.Build()
			require.NoError(t, err)

			client := &testReassignmentOCMClient{
				cluster:  ClusterInfo{HCPClusterID: "hcp-id", MgmtClusterID: "source-id", MgmtClusterName: "source-mc"},
				clusters: map[string]*cmv1.Cluster{"source-id": source, "replacement-id": replacement},
			}
			plan, err := NewDefaultReassignmentManager(client).PrepareReassignment(context.Background(), ReassignmentRequest{ClusterID: "hcp-id", ReplacementMC: "replacement-id"})
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "source-mc", plan.SourceManagementCluster)
			assert.Equal(t, "replacement-mc", plan.ReplacementManagementCluster)
		})
	}
}

func TestReassign(t *testing.T) {
	updateErr := errors.New("connection reset")
	readErr := errors.New("connection reset while reading")
	for _, tt := range []struct {
		name            string
		assignments     []assignmentResponse
		updateErrs      []error
		wantUpdates     []string
		alreadyAssigned bool
		wantErr         string
		wantUpdateErr   error
	}{
		{name: "already assigned", assignments: []assignmentResponse{{assignment: "replacement-mc"}}, alreadyAssigned: true},
		{name: "stale assignment", assignments: []assignmentResponse{{assignment: "other-mc"}}, wantErr: "changed from source-mc to other-mc"},
		{name: "success", assignments: []assignmentResponse{{assignment: "source-mc"}, {assignment: "replacement-mc"}}, wantUpdates: []string{"replacement-mc"}},
		{name: "read back mismatch", assignments: []assignmentResponse{{assignment: "source-mc"}, {assignment: "other-mc"}}, wantUpdates: []string{"replacement-mc"}, wantErr: "is other-mc after update"},
		{name: "read back error", assignments: []assignmentResponse{{assignment: "source-mc"}, {err: readErr}}, wantUpdates: []string{"replacement-mc"}, wantErr: "reading management cluster assignment after update", wantUpdateErr: readErr},
		{name: "update error", assignments: []assignmentResponse{{assignment: "source-mc"}}, updateErrs: []error{updateErr}, wantUpdates: []string{"replacement-mc"}, wantErr: "connection reset", wantUpdateErr: updateErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &testReassignmentOCMClient{assignments: tt.assignments, updateErrs: tt.updateErrs}
			plan := ReassignmentPlan{HCPClusterID: "hcp-id", SourceManagementCluster: "source-mc", ReplacementManagementCluster: "replacement-mc", ocmClient: client}
			result, err := NewDefaultReassignmentManager(client).Reassign(context.Background(), plan)
			assert.Equal(t, tt.wantUpdates, client.updates)
			assert.Equal(t, tt.alreadyAssigned, result.AlreadyAssigned)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
			if tt.wantUpdateErr != nil {
				assert.ErrorIs(t, err, tt.wantUpdateErr)
			}
		})
	}
}
