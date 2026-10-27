package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/smithy-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	awsmock "github.com/openshift/osdctl/pkg/provider/aws/mock"
	logrus "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSnapshotCopyTagsIncludeMeaningfulName(t *testing.T) {
	tags := snapshotCopyTags("cluster", "operation", "vsc", "destination")[0].Tags
	values := make(map[string]string, len(tags))
	for _, tag := range tags {
		values[awssdk.ToString(tag.Key)] = awssdk.ToString(tag.Value)
	}

	assert.Equal(t, "osdctl-hcp-cluster-vsc-destination", values["Name"])
	assert.Equal(t, "operation", values["osdctl.openshift.io/snapshot-copy"])
	assert.Equal(t, "vsc", values["osdctl.openshift.io/vsc"])
	assert.Equal(t, "destination", values["osdctl.openshift.io/stage"])
}

func TestSnapshotCopyNamePreservesStageWithinTagLimit(t *testing.T) {
	name := snapshotCopyName("cluster", strings.Repeat("v", 300), "intermediary")

	assert.Len(t, name, 256)
	assert.True(t, strings.HasSuffix(name, "-intermediary"))
}

func TestNewDefaultSnapshotCopierWithLogger(t *testing.T) {
	logger := logrus.New()

	copier := NewDefaultSnapshotCopierWithLogger(nil, logger)

	assert.Same(t, logger, copier.logger)
}

func TestCleanupSnapshotCopies(t *testing.T) {
	controller := gomock.NewController(t)
	source := awsmock.NewMockClient(controller)
	journal := snapshotCopyJournal{
		SourceCMKID:      "key",
		SourceCMKAlias:   "alias/hcp-migration",
		SourceCMKGrantID: "grant",
		Entries: []SnapshotCopyEntry{{
			VSCName:                "vsc",
			IntermediarySnapshotID: "snap-intermediary",
			DestinationSnapshotID:  "snap-destination",
		}},
	}
	writer := snapshotJournalWriter{path: filepath.Join(t.TempDir(), "journal.json"), journal: &journal}
	source.EXPECT().DeleteSnapshot(gomock.Any(), gomock.Any()).Return(&ec2.DeleteSnapshotOutput{}, nil)
	source.EXPECT().RevokeGrant(gomock.Any(), gomock.Any()).Return(&kms.RevokeGrantOutput{}, nil)
	source.EXPECT().DeleteAlias(gomock.Any(), gomock.Any()).Return(&kms.DeleteAliasOutput{}, nil)
	source.EXPECT().ScheduleKeyDeletion(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, input *kms.ScheduleKeyDeletionInput) (*kms.ScheduleKeyDeletionOutput, error) {
		assert.Equal(t, int32(7), awssdk.ToInt32(input.PendingWindowInDays))
		return &kms.ScheduleKeyDeletionOutput{}, nil
	})
	source.EXPECT().DescribeKey(gomock.Any(), gomock.Any()).Return(&kms.DescribeKeyOutput{KeyMetadata: &kmstypes.KeyMetadata{KeyState: kmstypes.KeyStatePendingDeletion}}, nil)

	err := cleanupSnapshotCopies(context.Background(), source, &writer, logrus.New())

	require.NoError(t, err)
	assert.True(t, journal.Entries[0].IntermediaryDeleted)
	assert.True(t, journal.SourceCMKGrantRevoked)
	assert.True(t, journal.SourceCMKAliasDeleted)
	assert.True(t, journal.SourceCMKDeletionScheduled)
}

func TestManagementClusterAccountID(t *testing.T) {
	cluster, err := cmv1.NewCluster().AWS(cmv1.NewAWS().STS(cmv1.NewSTS().SupportRoleARN("arn:aws:iam::123456789012:role/ManagedOpenShift-Support-Role"))).Build()
	require.NoError(t, err)

	accountID, err := managementClusterAccountID(cluster)
	require.NoError(t, err)
	assert.Equal(t, "123456789012", accountID)
}

func TestManagementClusterAccountIDRequiresSupportRole(t *testing.T) {
	cluster, err := cmv1.NewCluster().Build()
	require.NoError(t, err)

	_, err = managementClusterAccountID(cluster)
	assert.ErrorContains(t, err, "support role")
}

func TestSnapshotEntriesSelectsHCPNamespaceAndSorts(t *testing.T) {
	contents := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "second"}, "spec": map[string]any{"volumeSnapshotRef": map[string]any{"namespace": "ocm-int-2tn61a7fco018besk9lk86c531imgqjn-mcs-149"}, "driver": "ebs.csi.aws.com", "source": map[string]any{"snapshotHandle": "snap-second"}}}},
		{Object: map[string]any{"metadata": map[string]any{"name": "other"}, "spec": map[string]any{"volumeSnapshotRef": map[string]any{"namespace": "ocm-int-other"}, "driver": "ebs.csi.aws.com"}, "status": map[string]any{"snapshotHandle": "snap-other"}}},
		{Object: map[string]any{"metadata": map[string]any{"name": "first"}, "spec": map[string]any{"volumeSnapshotRef": map[string]any{"namespace": "ocm-int-2tn61a7fco018besk9lk86c531imgqjn-mcs-149"}, "driver": "ebs.csi.aws.com"}, "status": map[string]any{"snapshotHandle": "snap-first"}}},
	}}

	entries, err := snapshotEntries(contents, "ocm-int-2tn61a7fco018besk9lk86c531imgqjn-mcs-149")
	require.NoError(t, err)
	assert.Equal(t, []SnapshotCopyEntry{{VSCName: "first", OriginalSnapshotID: "snap-first"}, {VSCName: "second", OriginalSnapshotID: "snap-second"}}, entries)

	inventory := selectedVSCInventory(contents, entries)
	require.Len(t, inventory, 2)
	assert.Equal(t, []string{"second", "first"}, []string{inventory[0]["metadata"].(map[string]any)["name"].(string), inventory[1]["metadata"].(map[string]any)["name"].(string)})
}

func TestSnapshotJournalMustMatchOperation(t *testing.T) {
	path := filepath.Join(t.TempDir(), snapshotJournalName("hcp"))
	plan := SnapshotCopyPlan{ClusterID: "hcp", Region: "us-east-1", SourceAccountID: "source", ReplacementAccountID: "destination", Entries: []SnapshotCopyEntry{{VSCName: "vsc", OriginalSnapshotID: "snap-source"}}, VSCInventory: []map[string]any{{"metadata": map[string]any{"name": "vsc"}}}}
	journal, err := loadOrCreateSnapshotJournal(path, plan)
	require.NoError(t, err)
	assert.Equal(t, plan.VSCInventory, journal.VSCInventory)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var stored struct {
		Entries []SnapshotCopyEntry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(data, &stored))
	assert.Equal(t, plan.Entries, stored.Entries)

	plan.VSCInventory = []map[string]any{{"metadata": map[string]any{"name": "changed-vsc"}}}
	journal, err = loadOrCreateSnapshotJournal(path, plan)
	require.NoError(t, err)
	assert.Equal(t, "vsc", journal.VSCInventory[0]["metadata"].(map[string]any)["name"])

	plan.Region = "us-west-2"
	_, err = loadOrCreateSnapshotJournal(path, plan)
	assert.ErrorContains(t, err, "does not match")
}

func TestSnapshotJournalRejectsChangedSnapshotSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), snapshotJournalName("hcp"))
	plan := SnapshotCopyPlan{ClusterID: "hcp", Region: "us-east-1", SourceAccountID: "source", ReplacementAccountID: "destination", Entries: []SnapshotCopyEntry{{VSCName: "vsc", OriginalSnapshotID: "snap-source"}}}
	_, err := loadOrCreateSnapshotJournal(path, plan)
	require.NoError(t, err)

	plan.Entries = []SnapshotCopyEntry{{VSCName: "vsc", OriginalSnapshotID: "snap-replaced"}}
	_, err = loadOrCreateSnapshotJournal(path, plan)

	assert.ErrorContains(t, err, "does not match")
}

func TestSameSnapshotSources(t *testing.T) {
	current := []SnapshotCopyEntry{{VSCName: "first", OriginalSnapshotID: "snap-first"}, {VSCName: "second", OriginalSnapshotID: "snap-second"}}
	tests := []struct {
		name      string
		journaled []SnapshotCopyEntry
		matches   bool
	}{
		{name: "same entries in a different order", journaled: []SnapshotCopyEntry{{VSCName: "second", OriginalSnapshotID: "snap-second"}, {VSCName: "first", OriginalSnapshotID: "snap-first"}}, matches: true},
		{name: "added entry", journaled: append(current, SnapshotCopyEntry{VSCName: "third", OriginalSnapshotID: "snap-third"})},
		{name: "removed entry", journaled: current[:1]},
		{name: "changed source", journaled: []SnapshotCopyEntry{{VSCName: "first", OriginalSnapshotID: "snap-replaced"}, {VSCName: "second", OriginalSnapshotID: "snap-second"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.matches, sameSnapshotSources(tt.journaled, current))
		})
	}
}

func TestWaitForSnapshotRetriesUntilVisible(t *testing.T) {
	controller := gomock.NewController(t)
	client := awsmock.NewMockClient(controller)
	client.EXPECT().DescribeSnapshots(gomock.Any(), gomock.Any()).Return(nil, &smithy.GenericAPIError{Code: "InvalidSnapshot.NotFound"})
	client.EXPECT().DescribeSnapshots(gomock.Any(), gomock.Any()).Return(&ec2.DescribeSnapshotsOutput{}, nil)
	client.EXPECT().DescribeSnapshots(gomock.Any(), gomock.Any()).Return(&ec2.DescribeSnapshotsOutput{Snapshots: []ec2types.Snapshot{{State: ec2types.SnapshotStateCompleted}}}, nil)

	err := waitForSnapshot(context.Background(), client, "snap-123", 0)

	require.NoError(t, err)
}

func TestWaitForSnapshotReturnsFatalDescribeError(t *testing.T) {
	controller := gomock.NewController(t)
	client := awsmock.NewMockClient(controller)
	client.EXPECT().DescribeSnapshots(gomock.Any(), gomock.Any()).Return(nil, &smithy.GenericAPIError{Code: "UnauthorizedOperation"})

	err := waitForSnapshot(context.Background(), client, "snap-123", 0)

	assert.ErrorContains(t, err, "describing snapshot")
}

func TestWaitForSnapshotStopsAtContextDeadline(t *testing.T) {
	controller := gomock.NewController(t)
	client := awsmock.NewMockClient(controller)
	client.EXPECT().DescribeSnapshots(gomock.Any(), gomock.Any()).Return(&ec2.DescribeSnapshotsOutput{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForSnapshot(ctx, client, "snap-123", time.Hour)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunBoundedHonorsConcurrencyLimit(t *testing.T) {
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	done := make(chan error, 1)
	var active atomic.Int32

	go func() {
		done <- runBounded(context.Background(), 4, 2, func(context.Context, int) error {
			active.Add(1)
			started <- struct{}{}
			<-release
			active.Add(-1)
			return nil
		})
	}()

	<-started
	<-started
	assert.Equal(t, int32(2), active.Load())
	close(release)
	require.NoError(t, <-done)
}

func TestDefaultSnapshotJournalPath(t *testing.T) {
	t.Run("uses XDG data home", func(t *testing.T) {
		xdgDataHome := filepath.Join(t.TempDir(), "data")
		path, err := defaultSnapshotJournalPath(xdgDataHome, filepath.Join(t.TempDir(), "home"), "hcp")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(xdgDataHome, "osdctl", snapshotJournalName("hcp")), path)
		assert.DirExists(t, filepath.Dir(path))
	})

	t.Run("falls back to home data directory", func(t *testing.T) {
		directory := t.TempDir()
		unavailableXDGDataHome := filepath.Join(directory, "not-a-directory")
		require.NoError(t, os.WriteFile(unavailableXDGDataHome, nil, 0600))
		home := filepath.Join(directory, "home")

		path, err := defaultSnapshotJournalPath(unavailableXDGDataHome, home, "hcp")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(home, ".local", "share", "osdctl", snapshotJournalName("hcp")), path)
		assert.DirExists(t, filepath.Dir(path))
	})

	t.Run("falls back to current directory journal name", func(t *testing.T) {
		unavailableParent := filepath.Join(t.TempDir(), "not-a-directory")
		require.NoError(t, os.WriteFile(unavailableParent, nil, 0600))

		path, err := defaultSnapshotJournalPath(unavailableParent, unavailableParent, "hcp")
		require.NoError(t, err)
		assert.Equal(t, snapshotJournalName("hcp"), path)
	})
}

func TestAssumedRolePrincipal(t *testing.T) {
	tests := []struct {
		name      string
		callerARN string
		principal string
		wantErr   string
	}{
		{name: "commercial", callerARN: "arn:aws:sts::123456789012:assumed-role/role/session", principal: "arn:aws:iam::123456789012:role/role"},
		{name: "GovCloud", callerARN: "arn:aws-us-gov:sts::123456789012:assumed-role/role/session", principal: "arn:aws-us-gov:iam::123456789012:role/role"},
		{name: "China", callerARN: "arn:aws-cn:sts::123456789012:assumed-role/role/session", principal: "arn:aws-cn:iam::123456789012:role/role"},
		{name: "not assumed role", callerARN: "arn:aws:iam::123456789012:role/role", wantErr: "not an assumed-role ARN"},
		{name: "missing session", callerARN: "arn:aws:sts::123456789012:assumed-role/role", wantErr: "has no role"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			principal, err := assumedRolePrincipal(tt.callerARN)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.principal, principal)
		})
	}
}
