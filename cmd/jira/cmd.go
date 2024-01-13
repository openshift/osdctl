package jira

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "jira",
	Short: "Provides a set of commands for interacting with Jira",
	Args:  cobra.NoArgs,
}

func init() {
	Cmd.AddCommand(quickTaskCmd)
}
