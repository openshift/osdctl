package sts

import (
	"fmt"
	"github.com/spf13/cobra"
)

// NewCmdSts implements the STS utilities
func NewCmdSts() *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:               "sts",
		Short:             "STS related utilities",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	clusterCmd.AddCommand(newCmdPolicyDiff())
	clusterCmd.AddCommand(newCmdPolicy())
	return clusterCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Cannot print help")
		return
	}
}
