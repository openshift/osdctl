package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterAndReasonAddRequired(t *testing.T) {
	cmd := &cobra.Command{}
	cluster := new(Cluster)
	reason := new(Reason)

	cluster.AddRequired(cmd)
	reason.AddRequired(cmd, "Reason for elevated access")

	assert.Equal(t, "C", cmd.Flags().Lookup("cluster-id").Shorthand)
	assert.Equal(t, "true", cmd.Flags().Lookup("cluster-id").Annotations[cobra.BashCompOneRequiredFlag][0])
	assert.Equal(t, "true", cmd.Flags().Lookup("reason").Annotations[cobra.BashCompOneRequiredFlag][0])
}

func TestDestination(t *testing.T) {
	cmd := &cobra.Command{}
	destination := new(Destination)

	destination.Add(cmd, "Target management cluster")

	assert.NotNil(t, cmd.Flags().Lookup("target-mc"))
	assert.NotNil(t, cmd.Flags().Lookup("replacement-mc"))
	assert.Error(t, destination.Validate())
	*destination = "target"
	assert.NoError(t, destination.Validate())
}

func TestSourceAndAWSProfile(t *testing.T) {
	cmd := &cobra.Command{}
	source := new(Source)
	profile := new(AWSProfile)

	source.Add(cmd, "Source management cluster")
	profile.Add(cmd, "AWS profile")
	require.NoError(t, cmd.Flags().Parse([]string{"--source-mc", "source", "--profile", "profile"}))

	assert.Equal(t, Source("source"), *source)
	assert.Equal(t, AWSProfile("profile"), *profile)
}
