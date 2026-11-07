package backup

import (
	"testing"

	hcpflags "github.com/openshift/osdctl/cmd/hcp/internal/flags"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestBackupFlagsRegisterFlags(t *testing.T) {
	flags := &backupFlags{}
	cmd := &cobra.Command{}
	flags.RegisterFlags(cmd)

	err := cmd.Flags().Parse([]string{
		"--cluster-id", "abc123",
		"--reason", "OHSS-1234",
		"--label", "env=prod",
		"--annotation", "owner=sre-team",
	})

	assert.NoError(t, err)
	assert.Equal(t, hcpflags.Cluster("abc123"), flags.Cluster)
	assert.Equal(t, hcpflags.Reason("OHSS-1234"), flags.Reason)
	assert.Equal(t, map[string]string{"env": "prod"}, flags.labels)
	assert.Equal(t, map[string]string{"owner": "sre-team"}, flags.annotations)
}
