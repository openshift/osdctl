package cmd

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

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
	gitCommit := "unknown"

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				gitCommit = setting.Value
				break
			}
		}
	}

	latest, _ := utils.GetLatestVersion() // let's ignore this error, just in case we have no internet access
	ver, err := json.MarshalIndent(&versionResponse{
		Commit:  gitCommit,
		Version: utils.Version,
		Latest:  strings.TrimPrefix(latest, "v"),
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(ver))
	return nil
}
