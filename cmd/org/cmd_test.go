package org

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCmdOrg(t *testing.T) {
	cmd := NewCmdOrg()

	assert.NotNil(t, cmd)
	assert.Equal(t, "org", cmd.Use)
	assert.Equal(t, "Provides information for a specified organization", cmd.Short)

	subcommands := []string{
		"current",
		"get",
		"users",
		"labels",
		"describe",
		"clusters",
		"customers",
		"aws-accounts",
		"context orgId",
	}

	for _, subCmd := range subcommands {
		found := false
		for _, c := range cmd.Commands() {
			if c.Use == subCmd {
				found = true
				break
			}
		}
		assert.True(t, found, "Subcommand %s not found", subCmd)
	}
}
