package cmd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRestoreRequestValidate(t *testing.T) {
	valid := RestoreRequest{ClusterID: "cluster", BackupName: "backup", Reason: "OHSS-1234", Timeout: time.Minute}
	for _, tt := range []struct {
		name    string
		request RestoreRequest
		wantErr string
	}{
		{name: "valid", request: valid},
		{name: "missing cluster ID", request: RestoreRequest{BackupName: valid.BackupName, Reason: valid.Reason, Timeout: valid.Timeout}, wantErr: "cluster ID is required"},
		{name: "missing backup name", request: RestoreRequest{ClusterID: valid.ClusterID, Reason: valid.Reason, Timeout: valid.Timeout}, wantErr: "backup name is required"},
		{name: "missing reason", request: RestoreRequest{ClusterID: valid.ClusterID, BackupName: valid.BackupName, Timeout: valid.Timeout}, wantErr: "reason is required"},
		{name: "invalid timeout", request: RestoreRequest{ClusterID: valid.ClusterID, BackupName: valid.BackupName, Reason: valid.Reason}, wantErr: "timeout must be greater than zero"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}
