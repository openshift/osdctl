package policies

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractMajorMinor(t *testing.T) {
	tests := []struct {
		version string
		want    string
		wantErr bool
	}{
		{"4.17.0", "4.17", false},
		{"4.21.3", "4.21", false},
		{"5.0.0", "5.0", false},
		{"not-a-version", "", true},
		{"4.17", "", true}, // semver requires patch
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got, err := ExtractMajorMinor(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractMajorMinor(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractMajorMinor(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestCRToManagedPolicyKey(t *testing.T) {
	tests := []struct {
		namespace string
		name      string
		want      string
	}{
		{
			"openshift-cluster-csi-drivers",
			"ebs-cloud-credentials",
			"openshift_cluster_csi_drivers_ebs_cloud_credentials_policy",
		},
		{
			"openshift-cloud-ingress-operator",
			"cloud-credentials",
			"openshift_cloud_ingress_operator_cloud_credentials_policy",
		},
		{
			"openshift-machine-api",
			"aws-cloud-credentials",
			"openshift_machine_api_aws_cloud_credentials_policy",
		},
		{
			"openshift-image-registry",
			"installer-cloud-credentials",
			"openshift_image_registry_installer_cloud_credentials_policy",
		},
		{
			"openshift-ingress-operator",
			"cloud-credentials",
			"openshift_ingress_operator_cloud_credentials_policy",
		},
		{
			"openshift-cloud-network-config-controller",
			"cloud-credentials",
			"openshift_cloud_network_config_controller_cloud_credentials_policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.namespace+"/"+tt.name, func(t *testing.T) {
			got := CRToManagedPolicyKey(tt.namespace, tt.name)
			if got != tt.want {
				t.Errorf("CRToManagedPolicyKey(%q, %q) = %q, want %q",
					tt.namespace, tt.name, got, tt.want)
			}
		})
	}
}

func TestLoadPoliciesFromDir(t *testing.T) {
	dir := filepath.Join(testdataDir(), "policies")
	docs, err := LoadPoliciesFromDir(dir)
	if err != nil {
		t.Fatalf("LoadPoliciesFromDir(%q) failed: %v", dir, err)
	}

	if len(docs) == 0 {
		t.Error("expected at least one policy document")
	}

	if _, ok := docs["ROSAAmazonEBSCSIDriverOperatorPolicy"]; !ok {
		t.Errorf("expected ROSAAmazonEBSCSIDriverOperatorPolicy in loaded policies, got keys: %v",
			func() []string {
				var keys []string
				for k := range docs {
					keys = append(keys, k)
				}
				return keys
			}())
	}
}

func TestLoadPoliciesFromDir_Empty(t *testing.T) {
	dir := t.TempDir()
	docs, err := LoadPoliciesFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected empty map for empty directory, got %d entries", len(docs))
	}
}

func TestLoadPoliciesFromDir_NonExistent(t *testing.T) {
	_, err := LoadPoliciesFromDir("/nonexistent/path")
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestLoadPoliciesFromDir_SkipsNonJSON(t *testing.T) {
	dir := t.TempDir()

	// Create a valid JSON policy file
	policyJSON := `{"Version": "2012-10-17", "Statement": []}`
	if err := os.WriteFile(filepath.Join(dir, "test_policy.json"), []byte(policyJSON), 0600); err != nil {
		t.Fatalf("failed to write test policy file: %v", err)
	}

	// Create non-JSON files that should be skipped
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a policy"), 0600); err != nil {
		t.Fatalf("failed to write readme.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte("not a policy"), 0600); err != nil {
		t.Fatalf("failed to write manifest.yaml: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	docs, err := LoadPoliciesFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("expected 1 policy document, got %d", len(docs))
	}
	if _, ok := docs["test_policy"]; !ok {
		t.Error("expected test_policy in loaded policies")
	}
}
