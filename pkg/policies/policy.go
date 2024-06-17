package policies

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/coreos/go-semver/semver"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

// DownloadCredentialRequests creates a temp directory and extracts credential request
// manifests from a given release payload
func DownloadCredentialRequests(version string, cloud CloudSpec) (string, error) {
	directory, err := os.MkdirTemp("", "osdctl-crs-")
	if err != nil {
		return "", err
	}

	// try parsing the value for released versions
	_, err = semver.NewVersion(version)
	if err == nil {
		version = fmt.Sprintf("quay.io/openshift-release-dev/ocp-release:%s-x86_64", version)
	}

	crs := fmt.Sprintf("oc adm release extract %s --credentials-requests --cloud=%s --to=%s", version, cloud.String(), directory)

	output, err := exec.Command("bash", "-c", crs).CombinedOutput() //#nosec G204 -- Subprocess launched with variable
	if err != nil {
		return "", fmt.Errorf("failed to run command '%s': %w - Output: %s", crs, err, output)
	}

	return directory, nil
}

func ParseCredentialsRequestsInDir(dir string) ([]*cco.CredentialsRequest, error) {
	credReqs := []*cco.CredentialsRequest{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, olderr error) error {

		if olderr != nil {
			return olderr
		}

		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}

		credFile, inerr := os.Open(path)
		if inerr != nil {
			return fmt.Errorf("Error opening yaml file for reading: %w", inerr)
		}

		defer credFile.Close()

		tmpCCO := cco.CredentialsRequest{}

		inerr = k8syaml.NewYAMLOrJSONDecoder(credFile, 1024).Decode(&tmpCCO)

		if inerr != nil {
			return fmt.Errorf("Error parsing cco: %w", inerr)
		}

		credReqs = append(credReqs, &tmpCCO)
		return nil
	})
	return credReqs, err
}
