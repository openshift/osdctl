package restore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRestoreLeafFlags(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		flags []string
	}{
		{name: "create", cmd: "create", flags: []string{"cluster-id", "target-mc", "backup-name", "reason", "timeout"}},
		{name: "release DNS", cmd: "release-dns", flags: []string{"cluster-id", "profile"}},
		{name: "trust policy", cmd: "update-trust-policy", flags: []string{"cluster-id", "source-mc", "replacement-mc", "profile"}},
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
