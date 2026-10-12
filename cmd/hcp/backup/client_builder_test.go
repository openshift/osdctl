package backup

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServiceClusterNameFromServer(t *testing.T) {
	tests := []struct {
		name    string
		server  string
		want    string
		wantErr string
	}{
		{
			name:   "service cluster endpoint",
			server: "https://api.hs-sc-abc123.example.com:6443",
			want:   "hs-sc-abc123",
		},
		{
			name:    "missing API subdomain",
			server:  "https://hs-sc-abc123.example.com",
			wantErr: "invalid HyperShift server URL",
		},
		{
			name:    "invalid URL",
			server:  "not a URL",
			wantErr: "invalid HyperShift server URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := serviceClusterNameFromServer(tt.server)

			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr, "expected invalid server URL to return a generic validation error")
				assert.NotContains(t, err.Error(), tt.server, "validation error must not expose the service-cluster endpoint")
				return
			}
			assert.NoError(t, err, "expected service-cluster endpoint to parse")
			assert.Equal(t, tt.want, got, "expected service-cluster name from endpoint")
		})
	}
}
