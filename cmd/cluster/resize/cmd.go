package resize

import (
	"github.com/spf13/cobra"
)

var supportedInstanceTypes = map[string][]string{
	"controlplane": {"m5.4xlarge", "m5.8xlarge", "m5.12xlarge", "m5.16xlarge", "m5.24xlarge"},
	"infra":        {"r5.4xlarge", "r5.8xlarge", "r5.12xlarge", "r5.16xlarge", "r5.24xlarge"},
}

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
