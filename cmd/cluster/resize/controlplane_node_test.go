package resize

import (
	"strings"
	"testing"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

func TestValidateAWSInstanceTypeChange(t *testing.T) {
	tests := []struct {
		name                string
		currentInstanceType string
		newInstanceType     string
		errContains         string
	}{
		{
			name:                "same general purpose family",
			currentInstanceType: "m5.2xlarge",
			newInstanceType:     "m5.4xlarge",
		},
		{
			name:                "m5 to m6i",
			currentInstanceType: "m5.4xlarge",
			newInstanceType:     "m6i.4xlarge",
		},
		{
			name:                "unsupported requested instance type",
			currentInstanceType: "m5.4xlarge",
			newInstanceType:     "m6i.not-a-size",
			errContains:         "instance type m6i.not-a-size not supported for controlplane nodes",
		},
		{
			name:                "m6i to m5 is not allowed",
			currentInstanceType: "m6i.4xlarge",
			newInstanceType:     "m5.4xlarge",
			errContains:         "cannot change instance family from m6i to m5",
		},
		{
			name:                "other family change is not allowed",
			currentInstanceType: "m5.4xlarge",
			newInstanceType:     "r5.4xlarge",
			errContains:         "cannot change instance family from m5 to r5",
		},
		{
			name:                "invalid current instance type",
			currentInstanceType: "m5",
			newInstanceType:     "m6i.4xlarge",
			errContains:         "instance type m5 is not a valid instance type",
		},
		{
			name:                "missing instance size",
			currentInstanceType: "m5.",
			newInstanceType:     "m6i.4xlarge",
			errContains:         "instance type m5. is not a valid instance type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAWSInstanceTypeChange(tt.currentInstanceType, tt.newInstanceType)
			if tt.errContains == "" {
				if err != nil {
					t.Fatalf("validateAWSInstanceTypeChange() error = %v", err)
				}
				return
			}

			if err == nil || !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("validateAWSInstanceTypeChange() error = %v, want error containing %q", err, tt.errContains)
			}
		})
	}
}

func TestErrIfHostedControlPlane(t *testing.T) {
	tests := []struct {
		name      string
		cluster   *cmv1.Cluster
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "nil cluster",
			cluster: nil,
		},
		{
			name:    "classic cluster",
			cluster: newTestCluster(t, cmv1.NewCluster()),
		},
		{
			name:    "hypershift disabled",
			cluster: newTestCluster(t, cmv1.NewCluster().Hypershift(cmv1.NewHypershift().Enabled(false))),
		},
		{
			name:      "hcp cluster",
			cluster:   newTestCluster(t, cmv1.NewCluster().Hypershift(cmv1.NewHypershift().Enabled(true))),
			wantErr:   true,
			errSubstr: "should not be used for HCP clusters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errIfHostedControlPlane(tt.cluster)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("errIfHostedControlPlane() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("errIfHostedControlPlane() error = %v, want error containing %q", err, tt.errSubstr)
			}
		})
	}
}

func TestFormatControlPlaneResizePrompt(t *testing.T) {
	t.Run("customer cluster has identity and no warning banner", func(t *testing.T) {
		got := formatControlPlaneResizePrompt("prod-customer-01", "abc123", "m5.4xlarge", "m6i.4xlarge", "aws", "")
		for _, want := range []string{
			"Cluster        : prod-customer-01 (abc123)",
			"Type           : classic OSD/ROSA",
			"Cloud          : aws",
			"Current type   : m5.4xlarge",
			"Target type    : m6i.4xlarge",
			"This process runs asynchronously.",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("prompt missing %q\n%s", want, got)
			}
		}
		if strings.Contains(got, "WARNING:") {
			t.Errorf("customer prompt should not contain WARNING:\n%s", got)
		}
	})

	t.Run("management cluster shows infra warning", func(t *testing.T) {
		got := formatControlPlaneResizePrompt("hs-mc-773jpgko0", "mcid", "m5.4xlarge", "m6i.4xlarge", "aws", "Management")
		for _, want := range []string{
			"WARNING: You are about to resize a MANAGEMENT cluster",
			"Cluster        : hs-mc-773jpgko0 (mcid)",
			"Type           : MANAGEMENT",
			"Cloud          : aws",
			"Current type   : m5.4xlarge",
			"Target type    : m6i.4xlarge",
			"Risk Note: This operation targets an infrastructure cluster that",
			"underpins hosted control planes. Proceed with caution",
			"Have you confirmed this cluster ID/name is the intended target",
			"This process runs asynchronously.",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("prompt missing %q\n%s", want, got)
			}
		}
	})

	t.Run("hive cluster shows hive warning", func(t *testing.T) {
		got := formatControlPlaneResizePrompt("hivep01ue1", "hiveid", "m5.2xlarge", "m5.4xlarge", "aws", "Hive")
		if !strings.Contains(got, "WARNING: You are about to resize a HIVE cluster") {
			t.Errorf("prompt missing Hive warning\n%s", got)
		}
	})

	t.Run("service cluster shows service warning", func(t *testing.T) {
		got := formatControlPlaneResizePrompt("hs-sc-abc", "scid", "m5.2xlarge", "m5.4xlarge", "aws", "Service")
		if !strings.Contains(got, "WARNING: You are about to resize a SERVICE cluster") {
			t.Errorf("prompt missing Service warning\n%s", got)
		}
	})
}
