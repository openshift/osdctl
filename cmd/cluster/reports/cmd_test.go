package reports

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCmdReports(t *testing.T) {
	cmd := NewCmdReports()

	assert.NotNil(t, cmd)
	assert.Equal(t, "reports", cmd.Use)
	assert.Equal(t, "Cluster Reports from backplane-api", cmd.Short)

	// Check that subcommands are registered
	subcommands := cmd.Commands()
	assert.Len(t, subcommands, 3, "Reports command should have 3 subcommands")

	// Check for specific subcommands
	var hasListCmd, hasGetCmd, hasCreateCmd bool
	for _, subcmd := range subcommands {
		switch subcmd.Use {
		case "list":
			hasListCmd = true
		case "get":
			hasGetCmd = true
		case "create":
			hasCreateCmd = true
		}
	}

	assert.True(t, hasListCmd, "Reports command should have 'list' subcommand")
	assert.True(t, hasGetCmd, "Reports command should have 'get' subcommand")
	assert.True(t, hasCreateCmd, "Reports command should have 'create' subcommand")
}
