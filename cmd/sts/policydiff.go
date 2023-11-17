package sts

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type policyDiffOptions struct {
	oldReleaseVersion string
	newReleaseVersion string
}

func newCmdPolicyDiff() *cobra.Command {
	ops := &policyDiffOptions{}
	policyDiffCmd := &cobra.Command{
		Use:               "policy-diff",
		Short:             "Get diff between two versions of OCP STS policy",
		Args:              cobra.ExactArgs(2),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run(args))
		},
	}

	policyDiffCmd.Flags().StringVarP(&ops.oldReleaseVersion, "previous-version", "p", "", "")
	policyDiffCmd.Flags().StringVarP(&ops.newReleaseVersion, "new-version", "n", "", "")

	return policyDiffCmd
}

func (o *policyDiffOptions) complete(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return cmdutil.UsageErrorf(cmd, "Previous and new release version is required for policy-diff command")
	}

	return nil
}

func (o *policyDiffOptions) run(args []string) error {
	dir1, err := getPolicyFiles(args[0])
	if err != nil {
		return err
	}

	dir2, err := getPolicyFiles(args[1])
	if err != nil {
		return err
	}

	diff := fmt.Sprintf("diff %s %s", dir1, dir2)
	output, _ := exec.Command("bash", "-c", diff).Output() //#nosec G204 -- Subprocess launched with variable
	fmt.Println(string(output))

	return nil
}
