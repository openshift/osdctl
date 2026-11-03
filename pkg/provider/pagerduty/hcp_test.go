package pagerduty

import "testing"

func TestRhobsCellName(t *testing.T) {
	name, err := rhobsCellName("https://us-east-1-0.rhobs.api.openshift.com")
	if err != nil {
		t.Fatal(err)
	}
	if name != "us-east-1-0" {
		t.Fatalf("rhobsCellName() = %q, want %q", name, "us-east-1-0")
	}
}

func TestAlertDetailsContainClusterID(t *testing.T) {
	body := map[string]interface{}{
		"details": map[string]interface{}{
			"cluster_id": "cluster-external-uuid",
		},
	}
	if !alertDetailsContainClusterID(body, "cluster-external-uuid") {
		t.Fatal("alert details did not match the cluster external UUID")
	}
	if alertDetailsContainClusterID(body, "different-cluster") {
		t.Fatal("alert details matched the wrong cluster external UUID")
	}

	legacyBody := map[string]interface{}{
		"custom_details": map[string]interface{}{
			"external_cluster_id": "cluster-external-uuid",
		},
	}
	if !alertDetailsContainClusterID(legacyBody, "cluster-external-uuid") {
		t.Fatal("legacy custom details did not match the cluster external UUID")
	}
}

func TestHCPServiceNameMatches(t *testing.T) {
	tests := []struct {
		name        string
		region      string
		environment string
		severity    string
		want        bool
	}{
		{"rhobs-hcp-prod-critical-useast1", "us-east-1", "production", "critical", true},
		{"rhobs-hcp-prod-warning-useast1", "us-east-1", "production", "warning", true},
		{"[HCP] [RHOBS] Production Critical us-east-1", "us-east-1", "production", "critical", true},
		{"rhobs-hcp-stage-useast1", "us-east-1", "stage", "", true},
		{"rhobs-hcp-stage-critical-us-east-1", "us-east-1", "stage", "critical", false},
		{"platform-infrastructure-critical-us-east-1", "us-east-1", "production", "critical", false},
		{"rhobs-hcp-production-critical-us-west-2", "us-east-1", "production", "critical", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hcpServiceNameMatches(tt.name, tt.region, tt.environment, tt.severity); got != tt.want {
				t.Fatalf("hcpServiceNameMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}
