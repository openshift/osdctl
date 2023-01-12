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
)

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
