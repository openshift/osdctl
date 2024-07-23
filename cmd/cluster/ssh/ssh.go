package ssh

import "github.com/spf13/cobra"

func NewCmdSSH() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh",
		Short: "utilities for accessing cluster via ssh",
	}

	cmd.AddCommand(NewCmdKey())
	return cmd
}
