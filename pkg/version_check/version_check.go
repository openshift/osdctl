package version_check

import (
	"fmt"
	"os"
	"strings"

	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/slice"
)

// A list of command exempt from version checking
var exemptCommands = []string{
	"upgrade",
	"version",
}

// Runs the Version Check, if necessary
func Run(cmd *cobra.Command) error {
	// We are explicitly not reading this from viper because we do not want to persistently override this
	skipVersionCheck, err := cmd.Flags().GetBool("skip-version-check")
	if err != nil {
		return fmt.Errorf("flag --skip-version-check/-S undefined")
	}

	if shouldRunVersionCheck(skipVersionCheck, cmd.Use) {
		latestVersion, err := utils.GetLatestVersion()
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "WARN: Unable to verify that osdctl is running under the latest released version. Error trying to reach GitHub:")
			_, _ = fmt.Fprintln(os.Stderr, err)
			_, _ = fmt.Fprintln(os.Stderr, "Please be aware that you are possibly running an outdated or unreleased version.")
		}

		latestVersionTrimmed := trimVersionString(latestVersion)
		currentVersionTrimmed := trimVersionString(utils.Version)

		if latestVersionTrimmed != currentVersionTrimmed {
			_, _ = fmt.Fprintf(os.Stderr, "WARN: The current version (%s) is different than the latest released version (%s). It is recommended that you update to the latest released version to ensure that no known bugs or issues are hit.\n", utils.Version, latestVersion)

			if !utils.ConfirmPrompt() {
				return fmt.Errorf("user exited")
			}
		}
	}

	return nil
}

// Checks if the version check should be run
func shouldRunVersionCheck(skipVersionCheckFlag bool, commandName string) bool {

	// If either are true, then the version check should NOT run, hence negation
	return !(skipVersionCheckFlag || canCommandSkipVersionCheck(commandName))
}

func canCommandSkipVersionCheck(commandName string) bool {
	// Checks if the specific command is in the allowlist
	return slice.ContainsString(exemptCommands, commandName, nil)
}

func trimVersionString(version string) string {
	version = strings.TrimPrefix(version, "v")
	version, _, _ = strings.Cut(version, "-")
	return version
}
