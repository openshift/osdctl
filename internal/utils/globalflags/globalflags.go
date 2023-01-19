package globalflags

import (
	"flag"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/utils/pointer"
)

// GlobalOptions defines all available commands
type GlobalOptions struct {
	Output           string
	SkipVersionCheck bool
}

// AddGlobalFlags adds the Global Flags to the root command
func AddGlobalFlags(cmd *cobra.Command, opts *GlobalOptions) {
	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	cmd.PersistentFlags().StringVarP(&opts.Output, "output", "o", "", "Valid formats are ['', 'json', 'yaml', 'env']")
	cmd.PersistentFlags().BoolVarP(&opts.SkipVersionCheck, "skip-version-check", "S", false, "skip checking to see if this is the most recent release")
}

// GetFlags adds the kubeFlags we care about and adds the flags from the provided command
func GetFlags(cmd *cobra.Command) *genericclioptions.ConfigFlags {
	// Reuse kubectl global flags to provide namespace, context and credential options.
	// We are not using NewConfigFlags here to avoid adding too many flags
	flags := &genericclioptions.ConfigFlags{
		KubeConfig:  pointer.StringPtr(""),
		ClusterName: pointer.StringPtr(""),
		Context:     pointer.StringPtr(""),
		APIServer:   pointer.StringPtr(""),
		Timeout:     pointer.StringPtr("0"),
		Insecure:    pointer.BoolPtr(false),
		Impersonate: pointer.StringPtr(""),
	}
	flags.AddFlags(cmd.PersistentFlags())
	return flags
}
