package common

import (
	"testing"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

// TestGetKubeConfigAndClientWithConn tests the GetKubeConfigAndClientWithConn function
// which creates a Kubernetes client, REST config, and clientset using a provided OCM SDK
// connection. This function supports both regular and elevated (backplane-cluster-admin)
// access based on the presence of elevation reasons.
func TestGetKubeConfigAndClientWithConn(t *testing.T) {
	tests := []struct {
		name             string
		clusterID        string
		ocmConn          *sdk.Connection
		elevationReasons []string
		wantErr          bool
	}{
		{
			// Test that passing a nil OCM connection without elevation returns an error
			name:             "nil OCM connection",
			clusterID:        "test-cluster-id",
			ocmConn:          nil,
			elevationReasons: nil,
			wantErr:          true,
		},
		{
			// Test that passing a nil OCM connection with elevation reasons also returns an error
			name:             "nil OCM connection with elevation reasons",
			clusterID:        "test-cluster-id",
			ocmConn:          nil,
			elevationReasons: []string{"testing"},
			wantErr:          true,
		},
		{
			// Test that passing an empty cluster ID with nil connection returns an error
			name:             "empty cluster ID with nil connection",
			clusterID:        "",
			ocmConn:          nil,
			elevationReasons: nil,
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeCli, kubeconfig, clientset, err := GetKubeConfigAndClientWithConn(tt.clusterID, tt.ocmConn, tt.elevationReasons...)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetKubeConfigAndClientWithConn() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("GetKubeConfigAndClientWithConn() unexpected error = %v", err)
				}
				if kubeCli == nil {
					t.Errorf("GetKubeConfigAndClientWithConn() returned nil kubeCli")
				}
				if kubeconfig == nil {
					t.Errorf("GetKubeConfigAndClientWithConn() returned nil kubeconfig")
				}
				if clientset == nil {
					t.Errorf("GetKubeConfigAndClientWithConn() returned nil clientset")
				}
			}
		})
	}
}
