package k8s

import (
	"testing"

	sdk "github.com/openshift-online/ocm-sdk-go"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestNewWithConn tests the NewWithConn function which creates a Kubernetes client
// using backplane with a provided OCM SDK connection. This allows connecting to clusters
// in different OCM environments by providing a custom OCM connection.
func TestNewWithConn(t *testing.T) {
	tests := []struct {
		name      string
		clusterID string
		options   client.Options
		ocmConn   *sdk.Connection
		wantErr   bool
	}{
		{
			// Test that passing a nil OCM connection returns an error
			name:      "nil OCM connection",
			clusterID: "test-cluster-id",
			options:   client.Options{},
			ocmConn:   nil,
			wantErr:   true,
		},
		{
			// Test that passing an empty cluster ID with nil connection returns an error
			name:      "empty cluster ID with nil connection",
			clusterID: "",
			options:   client.Options{},
			ocmConn:   nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewWithConn(tt.clusterID, tt.options, tt.ocmConn)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewWithConn() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("NewWithConn() unexpected error = %v", err)
				}
				if client == nil {
					t.Errorf("NewWithConn() returned nil client")
				}
			}
		})
	}
}

// TestNewAsBackplaneClusterAdminWithConn tests the NewAsBackplaneClusterAdminWithConn function
// which creates an elevated Kubernetes client (backplane-cluster-admin) using a provided OCM
// SDK connection. This function allows connecting to clusters in different OCM environments
// with elevated permissions.
func TestNewAsBackplaneClusterAdminWithConn(t *testing.T) {
	tests := []struct {
		name             string
		clusterID        string
		options          client.Options
		ocmConn          *sdk.Connection
		elevationReasons []string
		wantErr          bool
	}{
		{
			// Test that passing a nil OCM connection with elevation reasons returns an error
			name:             "nil OCM connection",
			clusterID:        "test-cluster-id",
			options:          client.Options{},
			ocmConn:          nil,
			elevationReasons: []string{"testing"},
			wantErr:          true,
		},
		{
			// Test that passing a nil OCM connection without elevation reasons also returns an error
			name:             "nil OCM connection with no elevation reasons",
			clusterID:        "test-cluster-id",
			options:          client.Options{},
			ocmConn:          nil,
			elevationReasons: nil,
			wantErr:          true,
		},
		{
			// Test that passing an empty cluster ID with nil connection returns an error
			name:             "empty cluster ID with nil connection",
			clusterID:        "",
			options:          client.Options{},
			ocmConn:          nil,
			elevationReasons: []string{"testing"},
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewAsBackplaneClusterAdminWithConn(tt.clusterID, tt.options, tt.ocmConn, tt.elevationReasons...)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewAsBackplaneClusterAdminWithConn() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("NewAsBackplaneClusterAdminWithConn() unexpected error = %v", err)
				}
				if client == nil {
					t.Errorf("NewAsBackplaneClusterAdminWithConn() returned nil client")
				}
			}
		})
	}
}
