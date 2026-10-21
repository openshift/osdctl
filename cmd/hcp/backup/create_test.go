package backup

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestBackupFlagsAddFlags(t *testing.T) {
	flags := &backupFlags{}
	set := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.AddFlags(set)

	err := set.Parse([]string{
		"--cluster-id", "abc123",
		"--reason", "OHSS-1234",
		"--label", "env=prod",
		"--annotation", "owner=sre-team",
	})

	assert.NoError(t, err)
	assert.Equal(t, "abc123", flags.clusterID)
	assert.Equal(t, "OHSS-1234", flags.reason)
	assert.Equal(t, map[string]string{"env": "prod"}, flags.labels)
	assert.Equal(t, map[string]string{"owner": "sre-team"}, flags.annotations)
}
