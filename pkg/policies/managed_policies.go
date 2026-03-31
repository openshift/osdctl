package policies

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/coreos/go-semver/semver"
)

const managedClusterConfigRepo = "https://github.com/openshift/managed-cluster-config.git"

// ExtractMajorMinor extracts the major.minor version string from a semver version.
// For example, "4.17.0" returns "4.17".
func ExtractMajorMinor(version string) (string, error) {
	v, err := semver.NewVersion(version)
	if err != nil {
		return "", fmt.Errorf("failed to parse version %q as semver: %w", version, err)
	}
	return fmt.Sprintf("%d.%d", v.Major, v.Minor), nil
}

// DownloadManagedPolicies clones the managed-cluster-config repository using a
// sparse checkout and returns the path to the STS policy directory for the given
// version. The version should be a full semver (e.g. "4.17.0"); major.minor is
// extracted automatically.
func DownloadManagedPolicies(version string) (string, error) {
	majorMinor, err := ExtractMajorMinor(version)
	if err != nil {
		return "", err
	}

	dir, err := os.MkdirTemp("", "osdctl-managed-policies-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Shallow clone with sparse checkout for just the STS policies
	cloneCmd := fmt.Sprintf("git clone --depth 1 --filter=blob:none --sparse %s %s",
		managedClusterConfigRepo, dir)
	output, err := exec.Command("bash", "-c", cloneCmd).CombinedOutput() //#nosec G204 -- Subprocess launched with variable
	if err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("failed to clone managed-cluster-config: %w - Output: %s", err, output)
	}

	sparseCmd := fmt.Sprintf("cd %s && git sparse-checkout set resources/sts/%s", dir, majorMinor)
	output, err = exec.Command("bash", "-c", sparseCmd).CombinedOutput() //#nosec G204 -- Subprocess launched with variable
	if err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("failed to sparse-checkout STS policies: %w - Output: %s", err, output)
	}

	policyDir := filepath.Join(dir, "resources", "sts", majorMinor)
	if _, err := os.Stat(policyDir); os.IsNotExist(err) {
		os.RemoveAll(dir)
		return "", fmt.Errorf("no STS policies found for version %s in managed-cluster-config", majorMinor)
	}

	return policyDir, nil
}

// LoadPoliciesFromDir reads all JSON policy files from a directory and returns
// them as a map keyed by filename without the .json extension.
func LoadPoliciesFromDir(dir string) (map[string]string, error) {
	docs := make(map[string]string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		doc, err := LoadPolicyDocument(path)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		docs[name] = doc
	}

	return docs, nil
}

// CRToManagedPolicyKey derives the managed-cluster-config policy filename key
// from a CredentialsRequest's secret ref namespace and name.
//
// The managed-cluster-config STS policy files follow the naming convention:
//
//	<namespace>_<secret-name>_policy.json
//
// with hyphens replaced by underscores. For example:
//
//	namespace: "openshift-cluster-csi-drivers", name: "ebs-cloud-credentials"
//	-> "openshift_cluster_csi_drivers_ebs_cloud_credentials_policy"
func CRToManagedPolicyKey(secretRefNamespace, crName string) string {
	ns := strings.ReplaceAll(secretRefNamespace, "-", "_")
	name := strings.ReplaceAll(crName, "-", "_")
	return ns + "_" + name + "_policy"
}
