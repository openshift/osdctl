package dnsrecover

import (
	"github.com/spf13/cobra"
)

// NewCmdDNSRecover creates and returns the dns-recover command
func NewCmdDNSRecover() *cobra.Command {
	return newCmdDNSRecover()
}
