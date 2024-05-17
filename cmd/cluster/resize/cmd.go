package resize

import (
	"github.com/spf13/cobra"
)

func NewCmdResize() *cobra.Command {
	resize := &cobra.Command{
		Use:   "resize",
		Short: "resize control-plane/infra nodes",
		Args:  cobra.NoArgs,
	}

	resize.AddCommand(
		newCmdResizeInfra(),
		newCmdResizeControlPlane(),
	)

	return resize
}
