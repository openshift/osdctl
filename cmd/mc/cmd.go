package mc

import "github.com/spf13/cobra"

func NewCmdMC() *cobra.Command {
	mc := &cobra.Command{
		Use:  "mc",
		Args: cobra.NoArgs,
	}

	mc.AddCommand(newCmdList())

	return mc
}
