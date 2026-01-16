package forceupgrade

import (
	"github.com/spf13/cobra"
)

// NewCmdForceUpgrade creates and returns the force upgrade command
func NewCmdForceUpgrade() *cobra.Command {
	return newCmdForceUpgrade()
}
