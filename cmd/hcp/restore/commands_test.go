package restore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestoreLeafFlags(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		flags []string
	}{
		{name: "create", cmd: "create", flags: []string{"cluster-id", "target-mc", "backup-name", "reason", "timeout"}},
		{name: "release DNS", cmd: "release-dns", flags: []string{"cluster-id", "profile"}},
		{name: "reassign", cmd: "reassign", flags: []string{"cluster-id", "replacement-mc"}},
		{name: "trust policy", cmd: "update-trust-policy", flags: []string{"cluster-id", "source-mc", "replacement-mc", "profile"}},
		{name: "copy snapshots", cmd: "copy-snapshots", flags: []string{"cluster-id", "source-mc", "replacement-mc", "snapshot-journal", "reason", "profile", "timeout", "concurrency"}},
	}

	restore := NewCmdRestore()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, err := restore.Find([]string{tt.cmd})
			assert.NoError(t, err)
			for _, flag := range tt.flags {
				assert.NotNil(t, cmd.Flags().Lookup(flag))
			}
		})
	}
}

func TestNextSnapshotJournalPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot-copy-journal-hcp.json")
	require.NoError(t, os.WriteFile(path, nil, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(path), "snapshot-copy-journal-hcp-1.json"), nil, 0600))

	next, err := nextSnapshotJournalPath(path)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(filepath.Dir(path), "snapshot-copy-journal-hcp-2.json"), next)
}

func TestCopySnapshotsSnapshotJournalUsesDynamicDefault(t *testing.T) {
	cmd := NewCmdCopySnapshots()

	flag := cmd.Flags().Lookup("snapshot-journal")
	assert.NotNil(t, flag)
	assert.Empty(t, flag.DefValue)
	assert.Nil(t, cmd.Flags().Lookup("source-account-id"))
	assert.Nil(t, cmd.Flags().Lookup("replacement-account-id"))
}

func TestManifestWorkLeavesRequireOperationFlags(t *testing.T) {
	manifestWork := NewCmdManifestWork()
	for _, path := range [][]string{{"compare"}, {"move", "velero-resources"}, {"move", "cluster-resources"}} {
		cmd, _, err := manifestWork.Find(path)
		assert.NoError(t, err)
		for _, flag := range []string{"cluster-id", "replacement-mc", "reason"} {
			assert.NotNil(t, cmd.Flags().Lookup(flag))
			assert.Contains(t, cmd.Example, "--"+flag)
		}
	}
}
