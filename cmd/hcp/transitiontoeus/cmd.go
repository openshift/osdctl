package transitiontoeus

import (
	"github.com/spf13/cobra"
)

// NewCmdTransitionToEUS creates and returns the transition-to-eus command
func NewCmdTransitionToEUS() *cobra.Command {
	return newCmdTransitionToEUS()
}
