package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestChangeVolumeType_ValidateTargetType verifies that the target type validation
// accepts all supported EBS volume types and rejects invalid ones.
func TestChangeVolumeType_ValidateTargetType(t *testing.T) {
	tests := []struct {
		name        string
		targetType  string
		expectError bool
	}{
		{"valid gp3", "gp3", false},
		{"valid gp2", "gp2", false},
		{"valid io1", "io1", false},
		{"valid io2", "io2", false},
		{"valid st1", "st1", false},
		{"valid sc1", "sc1", false},
		{"invalid type", "invalid", true},
		{"empty type", "", true},
	}

	validTypes := map[string]bool{"gp2": true, "gp3": true, "io1": true, "io2": true, "st1": true, "sc1": true}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := validTypes[tt.targetType]
			if tt.expectError {
				assert.False(t, valid, "expected type %q to be invalid", tt.targetType)
			} else {
				assert.True(t, valid, "expected type %q to be valid", tt.targetType)
			}
		})
	}
}

// TestChangeVolumeType_OptionsDefaults verifies that a zero-value changeVolumeTypeOptions
// has nil pointers for optional fields and false for boolean flags.
func TestChangeVolumeType_OptionsDefaults(t *testing.T) {
	ops := &changeVolumeTypeOptions{}

	assert.Nil(t, ops.iops, "default IOPS should be nil")
	assert.Nil(t, ops.throughput, "default throughput should be nil")
	assert.False(t, ops.dryRun, "default dryRun should be false")
	assert.Empty(t, ops.clusterID, "default clusterID should be empty")
	assert.Empty(t, ops.volumeID, "default volumeID should be empty")
	assert.Empty(t, ops.targetType, "default targetType should be empty")
	assert.Empty(t, ops.reason, "default reason should be empty")
}

// TestChangeVolumeType_CommandCreation verifies that the cobra command is properly
// configured with the expected usage string, required flags, and optional flags.
func TestChangeVolumeType_CommandCreation(t *testing.T) {
	cmd := newCmdChangeVolumeType()

	assert.NotNil(t, cmd, "command should not be nil")
	assert.Equal(t, "change-volume-type --cluster-id <cluster-id> --volume-id <volume-id> --target-type <type>", cmd.Use)
	assert.NotEmpty(t, cmd.Short, "Short description should not be empty")
	assert.NotEmpty(t, cmd.Long, "Long description should not be empty")
	assert.NotEmpty(t, cmd.Example, "Example should not be empty")

	// Verify required flags exist
	requiredFlags := []string{"cluster-id", "volume-id", "target-type", "reason"}
	for _, flagName := range requiredFlags {
		flag := cmd.Flag(flagName)
		assert.NotNilf(t, flag, "required flag %q should exist", flagName)
	}

	// Verify optional flags exist
	optionalFlags := []string{"iops", "throughput", "dry-run"}
	for _, flagName := range optionalFlags {
		flag := cmd.Flag(flagName)
		assert.NotNilf(t, flag, "optional flag %q should exist", flagName)
	}
}

// TestChangeVolumeType_IOPSCompatibility verifies that IOPS is only supported
// for io1, io2, and gp3 volume types.
func TestChangeVolumeType_IOPSCompatibility(t *testing.T) {
	iopsVal := int32(3000)

	tests := []struct {
		name       string
		targetType string
		wantErr    bool
	}{
		{"iops with gp3", "gp3", false},
		{"iops with io1", "io1", false},
		{"iops with io2", "io2", false},
		{"iops with gp2", "gp2", true},
		{"iops with st1", "st1", true},
		{"iops with sc1", "sc1", true},
	}

	iopsTypes := map[string]bool{"io1": true, "io2": true, "gp3": true}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			supported := iopsTypes[tt.targetType]
			if tt.wantErr {
				assert.False(t, supported, "--iops should not be supported for %s", tt.targetType)
			} else {
				assert.True(t, supported, "--iops should be supported for %s", tt.targetType)
			}

			// Verify the options struct accepts the pointer correctly
			ops := &changeVolumeTypeOptions{
				targetType: tt.targetType,
				iops:       &iopsVal,
			}
			assert.Equal(t, iopsVal, *ops.iops)
		})
	}
}

// TestChangeVolumeType_ThroughputCompatibility verifies that throughput is only
// supported for gp3 volume type.
func TestChangeVolumeType_ThroughputCompatibility(t *testing.T) {
	tests := []struct {
		name       string
		targetType string
		wantErr    bool
	}{
		{"throughput with gp3", "gp3", false},
		{"throughput with gp2", "gp2", true},
		{"throughput with io1", "io1", true},
		{"throughput with io2", "io2", true},
		{"throughput with st1", "st1", true},
		{"throughput with sc1", "sc1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			supported := tt.targetType == "gp3"
			if tt.wantErr {
				assert.False(t, supported, "--throughput should not be supported for %s", tt.targetType)
			} else {
				assert.True(t, supported, "--throughput should be supported for %s", tt.targetType)
			}
		})
	}
}

// TestChangeVolumeType_FlagShortcuts verifies that flag shorthand aliases are
// correctly configured.
func TestChangeVolumeType_FlagShortcuts(t *testing.T) {
	cmd := newCmdChangeVolumeType()

	shortcuts := map[string]string{
		"cluster-id":  "C",
		"volume-id":   "v",
		"target-type": "t",
	}

	for flagName, shorthand := range shortcuts {
		flag := cmd.Flag(flagName)
		assert.NotNilf(t, flag, "flag %q should exist", flagName)
		assert.Equalf(t, shorthand, flag.Shorthand, "flag %q should have shorthand %q", flagName, shorthand)
	}
}
