package resize

import (
	"github.com/spf13/cobra"
)

func NewCmdResize() *cobra.Command {
	resize := &cobra.Command{
		Use:   "resize",
		Short: "resize infra nodes",
		Args:  cobra.NoArgs,
	}

	resize.AddCommand(
		newCmdResizeInfra(),
	)

	return resize
}
