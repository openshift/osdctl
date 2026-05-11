package utils

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

const (
	VersionAPIEndpoint     = "https://api.github.com/repos/openshift/osdctl/releases/latest"
	VersionAddressTemplate = "https://github.com/openshift/osdctl/releases/download/v%s/osdctl_%s_%s_%s.tar.gz" // version, version, GOOS, GOARCH
)

var (
	// GitCommit is the short git commit hash from the environment
	// Will be set during build process via GoReleaser
	// See also: https://pkg.go.dev/cmd/link
	GitCommit string

	// Version is the tag version from the environment
	// Will be set during build process via GoReleaser
	// See also: https://pkg.go.dev/cmd/link
	Version string

	// InstallMethod is set at build time via -X ldflags when osdctl is
	// built by a package manager. Empty string (default) means the binary
	// was built from source or via GoReleaser (GitHub releases).
	// Known values: "copr", "homebrew".
	InstallMethod string
)

// IsManagedInstall reports whether osdctl was installed via a package
// manager (e.g. COPR/RPM, Homebrew) rather than from a GitHub release.
func IsManagedInstall() bool {
	return InstallMethod != ""
}

// UpgradeInstruction returns a human-readable upgrade command for the
// current install method. Returns empty string for non-managed installs.
func UpgradeInstruction() string {
	switch InstallMethod {
	case "copr":
		return "dnf upgrade osdctl"
	case "homebrew":
		return "brew upgrade osdctl"
	default:
		return ""
	}
}

// githubResponse is a necessary struct for the JSON unmarshalling that is happening
// in the getLatestVersion().
type gitHubResponse struct {
	TagName string `json:"tag_name"`
}

// getLatestVersion connects to the GitHub API and returns the latest osdctl tag name
// Interesting Note: GitHub only shows the latest "stable" tag. This means, that
// tags with a suffix like *-rc.1 are not returned. We will always show the latest stable on master branch.
func GetLatestVersion() (latest string, err error) {
	client := http.Client{
		Timeout: time.Second * 10,
	}

	req, err := http.NewRequest(http.MethodGet, VersionAPIEndpoint, nil)
	if err != nil {
		return latest, err
	}

	res, err := client.Do(req)
	if err != nil {
		return latest, err
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return latest, err
	}

	githubResp := gitHubResponse{}
	err = json.Unmarshal(body, &githubResp)
	if err != nil {
		return latest, err
	}

	return githubResp.TagName, nil
}
