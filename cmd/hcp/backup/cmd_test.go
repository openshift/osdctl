package backup

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdBackup(t *testing.T) {
	cmd := NewCmdBackup()

	assert.Equal(t, "backup", cmd.Use)
	assert.Equal(t, "Manage Velero backups for HCP clusters", cmd.Short)
	assert.False(t, cmd.Runnable())

	assertCommandNames(t, cmd, "create", "discover")
}

func assertCommandNames(t *testing.T, cmd interface{ Commands() []*cobra.Command }, names ...string) {
	t.Helper()
	actual := make([]string, 0, len(cmd.Commands()))
	for _, subcommand := range cmd.Commands() {
		actual = append(actual, subcommand.Name())
	}
	assert.ElementsMatch(t, names, actual)
}
