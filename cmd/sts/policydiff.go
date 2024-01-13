package sts

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

type policyDiffOptions struct{}

func newCmdPolicyDiff() *cobra.Command {
	ops := &policyDiffOptions{}
	policyDiffCmd := &cobra.Command{
		Use:   "policy-diff",
		Short: "Get diff between two versions of OCP STS policy",
		Example: `
  # Compare 4.14.0 and 4.15.0-rc.0 AWS CloudCredentialRequests
  osdctl sts policy 4.14.0
  osdctl sts policy 4.15.0-rc.0
  osdctl sts policy-diff 4.14.0 4.15.0-rc.0
`,
		Args:              cobra.ExactArgs(2),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.run(args)
		},
	}

	return policyDiffCmd
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
