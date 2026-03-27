package evidence

import (
	"github.com/spf13/cobra"
)

// NewCmdEvidence returns the evidence command group
func NewCmdEvidence() *cobra.Command {
	evidenceCmd := &cobra.Command{
		Use:   "evidence",
		Short: "Evidence collection utilities for feature testing",
		Long: `Evidence collection utilities for feature testing.

This command group provides tools to help SRE teams collect evidence
during feature validation testing. The collected evidence can include
CloudTrail logs, cluster snapshots, and other diagnostic information.`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	evidenceCmd.AddCommand(newCmdCollect())

	return evidenceCmd
}
