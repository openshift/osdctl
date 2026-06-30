package cluster

import (
	"bytes"
	"testing"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestAuditPreRunE_MutualExclusivity(t *testing.T) {
	streams := genericclioptions.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}, In: nil}

	tests := []struct {
		name      string
		clusterID string
		accountID string
		setFlags  []string // which flags to explicitly set
		wantErr   string
	}{
		{
			name:    "neither provided",
			wantErr: "one of --cluster-id or --account-id is required",
		},
		{
			name:      "both provided",
			clusterID: "abc123",
			accountID: "def456",
			setFlags:  []string{"cluster-id", "account-id"},
			wantErr:   "mutually exclusive",
		},
		{
			name:      "cluster-id only",
			clusterID: "abc123",
			setFlags:  []string{"cluster-id"},
		},
		{
			name:      "account-id only",
			accountID: "def456",
			setFlags:  []string{"account-id"},
		},
		{
			name:      "account-id with special chars",
			accountID: "abc' OR 1=1 --",
			setFlags:  []string{"account-id"},
			wantErr:   "invalid characters",
		},
		{
			name:      "cluster-id with special chars",
			clusterID: "abc'; DROP TABLE",
			setFlags:  []string{"cluster-id"},
			wantErr:   "invalid characters",
		},
		{
			name:      "account-id with hyphens (valid)",
			accountID: "abc-123-def",
			setFlags:  []string{"account-id"},
		},
		{
			name:      "cluster-id with underscores (valid)",
			clusterID: "my_cluster_123",
			setFlags:  []string{"cluster-id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newCmdPullSecretAudit(streams, nil)

			for _, flag := range tt.setFlags {
				var val string
				switch flag {
				case "cluster-id":
					val = tt.clusterID
				case "account-id":
					val = tt.accountID
				}
				if err := cmd.Flags().Set(flag, val); err != nil {
					t.Fatalf("failed to set %s: %v", flag, err)
				}
			}
			if err := cmd.Flags().Set("reason", "test"); err != nil {
				t.Fatalf("failed to set reason: %v", err)
			}

			err := cmd.PreRunE(cmd, nil)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !bytes.Contains([]byte(err.Error()), []byte(tt.wantErr)) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}
