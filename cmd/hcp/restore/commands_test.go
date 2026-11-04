package restore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingWriter struct{ err error }

func (writer failingWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

func TestRestoreLeafFlags(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		flags []string
	}{
		{name: "create", cmd: "create", flags: []string{"cluster-id", "target-mc", "replacement-mc", "backup-name", "reason", "timeout"}},
		{name: "release DNS", cmd: "release-dns", flags: []string{"cluster-id", "profile"}},
		{name: "reassign", cmd: "reassign", flags: []string{"cluster-id", "target-mc", "replacement-mc"}},
		{name: "trust policy", cmd: "update-trust-policy", flags: []string{"cluster-id", "source-mc", "target-mc", "replacement-mc", "profile"}},
		{name: "migrate storage", cmd: "migrate-storage", flags: []string{"cluster-id", "source-mc", "target-mc", "replacement-mc", "snapshot-journal", "reason", "profile", "timeout", "concurrency"}},
		{name: "reimport", cmd: "reimport", flags: []string{"cluster-id", "target-mc", "replacement-mc", "reason", "timeout"}},
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

func TestRestoreFlagDefaults(t *testing.T) {
	tests := []struct {
		name        string
		command     *cobra.Command
		flag        string
		wantDefault string
	}{
		{name: "create timeout", command: NewCmdCreate(), flag: "timeout", wantDefault: "30m0s"},
		{name: "reimport timeout", command: NewCmdReimport(), flag: "timeout", wantDefault: "5m0s"},
		{name: "migrate timeout", command: NewCmdMigrateStorage(), flag: "timeout", wantDefault: "2h0m0s"},
		{name: "migrate concurrency", command: NewCmdMigrateStorage(), flag: "concurrency", wantDefault: "4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := tt.command.Flags().Lookup(tt.flag)
			require.NotNil(t, flag)
			assert.Equal(t, tt.wantDefault, flag.DefValue)
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

func TestMigrateStorageSnapshotJournalUsesDynamicDefault(t *testing.T) {
	cmd := NewCmdMigrateStorage()

	flag := cmd.Flags().Lookup("snapshot-journal")
	assert.NotNil(t, flag)
	assert.Empty(t, flag.DefValue)
	assert.Nil(t, cmd.Flags().Lookup("source-account-id"))
	assert.Nil(t, cmd.Flags().Lookup("replacement-account-id"))
}

func TestDiscoverSnapshotJournals(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "data[1]")
	require.NoError(t, os.Mkdir(directory, 0700))
	name := "snapshot-copy-journal-hcp.json"
	for _, path := range []string{
		filepath.Join(directory, name),
		filepath.Join(directory, "snapshot-copy-journal-hcp-2.json"),
		filepath.Join(directory, "snapshot-copy-journal-hcp-2-invalid.json"),
		filepath.Join(directory, "snapshot-copy-journal-hcp-other.json"),
		filepath.Join(directory, "snapshot-copy-journal-other.json"),
	} {
		require.NoError(t, os.WriteFile(path, nil, 0600))
	}

	paths, err := discoverSnapshotJournalsIn([]string{directory, directory}, "hcp")

	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(directory, name), filepath.Join(directory, "snapshot-copy-journal-hcp-2.json")}, paths)
}

func TestSelectSnapshotJournal(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "customer")
	require.NoError(t, os.Mkdir(directory, 0700))
	first := filepath.Join(directory, "snapshot-copy-journal-hcp.json")
	second := filepath.Join(directory, "snapshot-copy-journal-hcp-1.json")
	for _, path := range []string{first, second} {
		data, err := json.Marshal(map[string]any{"sourceAccountID": "123456789012", "replacementAccountID": "210987654321", "entries": []map[string]string{{"destinationSnapshotID": "snap"}}})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0600))
	}

	var out strings.Builder
	path, err := selectSnapshotJournalFromPaths(strings.NewReader("2\n"), &out, []string{first, second}, first)

	require.NoError(t, err)
	assert.Equal(t, second, path)
	assert.Contains(t, out.String(), "snapshot-copy-journal-hcp.json [directory \"customer\"] (account ending 9012 → account ending 4321, 1/1 copied)")
	assert.NotContains(t, out.String(), directory)
	assert.NotContains(t, out.String(), "123456789012")
	assert.NotContains(t, out.String(), "210987654321")
}

func TestSelectSnapshotJournalLabelsLocations(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	name := "snapshot-copy-journal-hcp.json"
	dataPath := filepath.Join(dataHome, "osdctl", name)
	require.NoError(t, os.Mkdir(filepath.Dir(dataPath), 0700))
	require.NoError(t, os.WriteFile(dataPath, nil, 0600))

	var out strings.Builder
	_, err := selectSnapshotJournalFromPaths(strings.NewReader("2\n"), &out, []string{name, dataPath}, name)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "snapshot-copy-journal-hcp.json [current directory]")
	assert.Contains(t, out.String(), "snapshot-copy-journal-hcp.json [XDG data directory]")
	assert.NotContains(t, out.String(), dataHome)
}

func TestRequestedSnapshotJournalPath(t *testing.T) {
	assert.Equal(t, "", requestedSnapshotJournalPath("", "resolved.json"))
	assert.Equal(t, "resolved.json", requestedSnapshotJournalPath("missing/../journal.json", "resolved.json"))
}

func TestSelectSnapshotJournalReturnsOutputError(t *testing.T) {
	errOutput := errors.New("output failed")
	_, err := selectSnapshotJournalFromPaths(strings.NewReader("2\n"), failingWriter{err: errOutput}, []string{"first", "second"}, "default")

	require.Error(t, err)
	assert.ErrorIs(t, err, errOutput)
}

func TestSelectSnapshotJournalStartsNewOperation(t *testing.T) {
	directory := t.TempDir()
	defaultPath := filepath.Join(directory, "snapshot-copy-journal-hcp.json")
	require.NoError(t, os.WriteFile(defaultPath, nil, 0600))

	path, err := selectSnapshotJournalFromPaths(strings.NewReader("no\n"), &strings.Builder{}, []string{defaultPath}, defaultPath)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(directory, "snapshot-copy-journal-hcp-1.json"), path)
}

func TestManifestWorkLeavesRequireOperationFlags(t *testing.T) {
	manifestWork := NewCmdManifestWork()
	for _, path := range [][]string{{"compare"}, {"move", "velero-resources"}, {"move", "cluster-resources"}} {
		cmd, _, err := manifestWork.Find(path)
		assert.NoError(t, err)
		for _, flag := range []string{"cluster-id", "target-mc", "replacement-mc", "reason"} {
			assert.NotNil(t, cmd.Flags().Lookup(flag))
		}
		for _, flag := range []string{"cluster-id", "target-mc", "reason"} {
			assert.Contains(t, cmd.Example, "--"+flag)
		}
	}
}
