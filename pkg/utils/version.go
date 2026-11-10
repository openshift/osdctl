package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
// current install method.
func UpgradeInstruction() (string, error) {
	switch InstallMethod {
	case "":
		return "", nil
	case "copr":
		return "dnf upgrade osdctl", nil
	case "homebrew":
		return "brew upgrade osdctl", nil
	default:
		return "", fmt.Errorf("unknown install method: %q", InstallMethod)
	}
}

type gitHubResponse struct {
	TagName     string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
}

// getLatestVersion connects to the GitHub API and returns the latest osdctl tag name
// Interesting Note: GitHub only shows the latest "stable" tag. This means, that
// tags with a suffix like *-rc.1 are not returned. We will always show the latest stable on master branch.
func GetLatestVersion() (latest string, err error) {
	var release gitHubResponse
	err = getReleaseJSON(&http.Client{Timeout: 10 * time.Second}, VersionAPIEndpoint, &release)
	return release.TagName, err
}

// CheckVersion also reports whether the current release qualifies for the
// one-week grace period following publication of its immediate successor.
func CheckVersion(current string) (latest string, gracePeriod bool, err error) {
	return checkVersion(&http.Client{Timeout: 10 * time.Second}, VersionAPIEndpoint, current, time.Now())
}

// checkVersion uses publication times to identify the previous stable release.
// Unverifiable eligibility never grants grace; the client and clock are supplied
// so boundary cases can be tested without contacting GitHub.
func checkVersion(client *http.Client, endpoint, current string, now time.Time) (string, bool, error) {
	var latest gitHubResponse
	if err := getReleaseJSON(client, endpoint, &latest); err != nil {
		return "", false, err
	}
	age := now.Sub(latest.PublishedAt)
	if strings.TrimPrefix(current, "v") == strings.TrimPrefix(latest.TagName, "v") ||
		latest.PublishedAt.IsZero() || age < 0 || age >= 7*24*time.Hour {
		return latest.TagName, false, nil
	}

	var releases []gitHubResponse
	if err := getReleaseJSON(client, strings.TrimSuffix(endpoint, "/latest")+"?per_page=100", &releases); err != nil {
		return latest.TagName, false, err
	}
	var previous gitHubResponse
	for _, release := range releases {
		if release.Draft || release.Prerelease || release.TagName == latest.TagName ||
			release.PublishedAt.IsZero() || !release.PublishedAt.Before(latest.PublishedAt) {
			continue
		}
		if release.PublishedAt.After(previous.PublishedAt) {
			previous = release
		}
	}
	return latest.TagName, previous.TagName != "" &&
		strings.TrimPrefix(current, "v") == strings.TrimPrefix(previous.TagName, "v"), nil
}

// getReleaseJSON rejects non-success responses before decoding so GitHub API
// errors cannot be mistaken for valid release metadata.
func getReleaseJSON(client *http.Client, endpoint string, result any) error {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub releases API returned %s", res.Status)
	}
	return json.NewDecoder(res.Body).Decode(result)
}
