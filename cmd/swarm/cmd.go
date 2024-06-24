package swarm

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "swarm",
	Short: "Provides a set of commands for swarming activity",
	Args:  cobra.NoArgs,
}

func init() {
	Cmd.AddCommand(secondaryCmd)
}
