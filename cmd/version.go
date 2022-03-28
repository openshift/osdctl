package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	versionAPIEndpoint     = "https://api.github.com/repos/openshift/osdctl/releases/latest"
	versionAddressTemplate = "https://github.com/openshift/osdctl/releases/download/v%s/osdctl_%s_%s_%s.tar.gz" // version, version, GOOS, GOARCH
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

// versionResponse is necessary for the JSON version response. It uses the three
// variables that get set during the build.
type versionResponse struct {
	Commit  string `json:"commit"`
	Version string `json:"version"`
	Latest  string `json:"latest"`
}

// versionCmd is the subcommand "osdctl version" for cobra.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display the version",
	Long:  "Display version of osdctl",
	RunE:  version,
}

// version returns the osdctl version marshalled in JSON
func version(cmd *cobra.Command, args []string) error {
	latest, _ := getLatestVersion() // let's ignore this error, just in case we have no internet access
	ver, err := json.MarshalIndent(&versionResponse{
		Commit:  GitCommit,
		Version: Version,
		Latest:  strings.TrimPrefix(latest, "v"),
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(ver))
	return nil
}

// getLatestVersion connects to the GitHub API and returns the latest osdctl tag name
// Interesting Note: GitHub only shows the latest "stable" tag. This means, that
// tags with a suffix like *-rc.1 are not returned. We will always show the latest stable on master branch.
func getLatestVersion() (latest string, err error) {
	client := http.Client{
		Timeout: time.Second * 2,
	}

	req, err := http.NewRequest(http.MethodGet, versionAPIEndpoint, nil)
	if err != nil {
		return latest, err
	}

	res, err := client.Do(req)
	if err != nil {
		return latest, err
	}

	body, err := ioutil.ReadAll(res.Body)
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
