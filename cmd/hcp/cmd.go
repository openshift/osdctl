package hcp

import (
	"github.com/openshift/osdctl/cmd/hcp/mustgather"
	"github.com/spf13/cobra"
)

func NewCmdHCP() *cobra.Command {
	hcp := &cobra.Command{
		Use:  "hcp",
		Args: cobra.NoArgs,
	}

	hcp.AddCommand(mustgather.NewCmdMustGather())

	return hcp
}
