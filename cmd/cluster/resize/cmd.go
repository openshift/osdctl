package resize

import (
	"github.com/spf13/cobra"
)

var supportedInstanceTypes = map[string][]string{
	"controlplane": {
		"m5.2xlarge",
		"m5.4xlarge",
		"m5.8xlarge",
		"m5.12xlarge",
		"m5.16xlarge",
		"m5.24xlarge",
		"m6i.2xlarge",
		"m6i.4xlarge",
		"m6i.8xlarge",
		"m6i.12xlarge",
		"m6i.16xlarge",
		"m6i.24xlarge",
		"m6i.32xlarge",
		"custom-8-32768",
		"custom-16-65536",
		"custom-32-131072",
		"n2-standard-8",
		"n2-standard-16",
		"n2-standard-32",
	},
	"infra": {
		"r5.xlarge",
		"r5.2xlarge",
		"r5.4xlarge",
		"r5.8xlarge",
		"r5.12xlarge",
		"r5.16xlarge",
		"r5.24xlarge",
		"custom-4-32768-ext",
		"custom-8-65536-ext",
		"custom-16-131072-ext",
		"n2-highmem-4",
		"n2-highmem-8",
		"n2-highmem-16",
	},
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
		newCmdResizeRequestServingNodes(),
	)

	return resize
}
