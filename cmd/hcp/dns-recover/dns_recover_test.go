package dnsrecover

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdDNSRecover(t *testing.T) {
	cmd := newCmdDNSRecover()

	assert.Equal(t, "recover", cmd.Name())
	assert.Contains(t, cmd.Short, "Recover ExternalDNS")
}

func TestNewCmdDNSRecover_RequiredFlags(t *testing.T) {
	cmd := newCmdDNSRecover()

	requiredFlags := []string{"cluster-id", "management-cluster", "reason"}
	for _, flag := range requiredFlags {
		f := cmd.Flags().Lookup(flag)
		require.NotNil(t, f, "expected required flag %q to exist", flag)
		ann := f.Annotations["cobra_annotation_bash_completion_one_required_flag"]
		assert.NotEmpty(t, ann, "expected flag %q to be marked required", flag)
	}
}

func TestNewCmdDNSRecover_OptionalFlags(t *testing.T) {
	cmd := newCmdDNSRecover()

	optionalFlags := []struct {
		name         string
		defaultValue string
	}{
		{"dry-run", "false"},
		{"skip-dns-verification", "false"},
		{"restart-network-components", "false"},
		{"debug", "false"},
	}

	for _, tt := range optionalFlags {
		t.Run(tt.name, func(t *testing.T) {
			f := cmd.Flags().Lookup(tt.name)
			require.NotNil(t, f, "expected optional flag %q to exist", tt.name)
			assert.Equal(t, tt.defaultValue, f.DefValue, "unexpected default for flag %q", tt.name)
		})
	}
}

func TestNewCmdDNSRecover_ShorthandFlags(t *testing.T) {
	cmd := newCmdDNSRecover()

	shorthands := map[string]string{
		"cluster-id":         "C",
		"management-cluster": "M",
	}

	for flag, shorthand := range shorthands {
		f := cmd.Flags().Lookup(flag)
		require.NotNil(t, f, "expected flag %q to exist", flag)
		assert.Equal(t, shorthand, f.Shorthand, "unexpected shorthand for flag %q", flag)
	}
}

func TestNewCmdDNSRecover_AcceptsNoArgs(t *testing.T) {
	cmd := newCmdDNSRecover()
	assert.NotNil(t, cmd.Args)
}

func TestValidate_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		opts    dnsRecoverOptions
		wantErr bool
	}{
		{
			name:    "all empty",
			opts:    dnsRecoverOptions{},
			wantErr: true,
		},
		{
			name: "missing management-cluster",
			opts: dnsRecoverOptions{
				clusterID: "test-cluster",
				reason:    "reason",
			},
			wantErr: true,
		},
		{
			name: "missing reason",
			opts: dnsRecoverOptions{
				clusterID:         "test-cluster",
				managementCluster: "dest-mc",
			},
			wantErr: true,
		},
		{
			name: "missing cluster-id",
			opts: dnsRecoverOptions{
				managementCluster: "dest-mc",
				reason:            "reason",
			},
			wantErr: true,
		},
		{
			name: "all provided",
			opts: dnsRecoverOptions{
				clusterID:         "test-cluster",
				managementCluster: "dest-mc",
				reason:            "reason",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := tt.opts.validate(ctx, nil)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAskConfirmation_FunctionExists(t *testing.T) {
	assert.NotNil(t, askConfirmation)
}
