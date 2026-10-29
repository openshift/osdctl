package restore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCmdRestore(t *testing.T) {
	cmd := NewCmdRestore()

	assert.Equal(t, "restore", cmd.Use)
	assert.Equal(t, "Prepare and restore HCP clusters during disaster recovery", cmd.Short)
	assert.False(t, cmd.Runnable())

	actual := make([]string, 0, len(cmd.Commands()))
	for _, subcommand := range cmd.Commands() {
		actual = append(actual, subcommand.Name())
	}
	assert.ElementsMatch(t, []string{"copy-snapshots", "create", "manifestwork", "reassign", "release-dns", "update-trust-policy"}, actual)
}
