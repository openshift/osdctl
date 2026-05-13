package ai

import (
	sreagent "github.com/openshift/osdctl/cmd/ai/sre_agent"
	"github.com/spf13/cobra"
)

// NewCmdAI implements the base AI command
func NewCmdAI() *cobra.Command {
	aiCmd := &cobra.Command{
		Use:   "ai",
		Short: "AI-powered tools for SRE automation",
		Args:  cobra.NoArgs,
	}

	aiCmd.AddCommand(sreagent.NewCmdSreAgent())

	return aiCmd
}
