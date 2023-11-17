package sts

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/coreos/go-semver/semver"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type policyOptions struct {
	ReleaseVersion string
}

func newCmdPolicy() *cobra.Command {
	ops := &policyOptions{}
	policyCmd := &cobra.Command{
		Use:               "policy",
		Short:             "Get OCP STS policy",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run(args))
		},
	}

	policyCmd.Flags().StringVarP(&ops.ReleaseVersion, "release-version", "r", "", "")

	return policyCmd
}

func (o *policyOptions) complete(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "Release version is required for policy command")
	}

	return nil
}

func (o *policyOptions) run(args []string) error {
	directory, err := getPolicyFiles(args[0])
	if err != nil {
		return err
	}

	output := "OCP STS policy files have been saved in " + directory + " directory"
	fmt.Println(output)

	return nil
}

// getPolicyFiles creates a temp directory and extracts credential request
// manifests from a given release payload
func getPolicyFiles(value string) (string, error) {
	directory, err := os.MkdirTemp("", "osdctl-crs-")
	if err != nil {
		return "", err
	}

	// try parsing the value for released versions
	_, err = semver.NewVersion(value)
	if err == nil {
		value = fmt.Sprintf("quay.io/openshift-release-dev/ocp-release:%s-x86_64", value)
	}

	crs := fmt.Sprintf("oc adm release extract %s --credentials-requests --cloud=aws --to=%s", value, directory)
	_, err = exec.Command("bash", "-c", crs).Output() //#nosec G204 -- Subprocess launched with variable
	if err != nil {
		return "", fmt.Errorf("failed to run command: %w", err)
	}

	return directory, nil
}
