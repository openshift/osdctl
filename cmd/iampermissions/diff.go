package iampermissions

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/openshift/osdctl/pkg/policies"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type diffOptions struct {
	BaseVersion   string
	TargetVersion string
	Cloud         policies.CloudSpec
}

const (
	baseVersionFlagName   = "base-version"
	targetVersionFlagName = "target-version"
)

func newCmdDiff() *cobra.Command {
	ops := &diffOptions{}
	policyCmd := &cobra.Command{
		Use:               "diff",
		Short:             "Diff iam permissions for cluster operators between two versions",
		Args:              cobra.ExactArgs(0),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ops.Cloud = *cmd.Flag(cloudFlagName).Value.(*policies.CloudSpec)
			cmdutil.CheckErr(ops.run())
		},
	}

	policyCmd.Flags().StringVarP(&ops.BaseVersion, baseVersionFlagName, "b", "", "")
	policyCmd.Flags().StringVarP(&ops.TargetVersion, targetVersionFlagName, "t", "", "")
	policyCmd.MarkFlagRequired(baseVersionFlagName)
	policyCmd.MarkFlagRequired(targetVersionFlagName)

	return policyCmd
}

func (o *diffOptions) run() error {
	fmt.Fprintf(os.Stderr, "Downloading Credential Requests for %s\n", o.BaseVersion)
	baseDir, err := policies.DownloadCredentialRequests(o.BaseVersion, o.Cloud)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Downloading Credential Requests for %s\n", o.TargetVersion)
	targetDir, err := policies.DownloadCredentialRequests(o.TargetVersion, o.Cloud)
	if err != nil {
		return err
	}

	output, _ := exec.Command("diff", baseDir, targetDir).CombinedOutput() //#nosec G204 -- Subprocess launched with variable
	fmt.Println(string(output))

	return nil
}
