package globalflags

import (
	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// GlobalOptions defines all available commands
type GlobalOptions struct {
	Output           string
	SkipVersionCheck bool
	NoAwsProxy       bool
}

// AddGlobalFlags adds the Global Flags to the root command
func AddGlobalFlags(cmd *cobra.Command, opts *GlobalOptions) {
	cmd.PersistentFlags().StringVarP(&opts.Output, "output", "o", "", "Valid formats are ['', 'json', 'yaml', 'env']")
	cmd.PersistentFlags().BoolVarP(&opts.SkipVersionCheck, "skip-version-check", "S", false, "skip checking to see if this is the most recent release")
	cmd.PersistentFlags().BoolVar(&opts.NoAwsProxy, aws.NoProxyFlag, false, "Don't use the configured `aws_proxy` value")
}

// GetFlags adds the kubeFlags we care about and adds the flags from the provided command
func GetFlags(cmd *cobra.Command) *genericclioptions.ConfigFlags {
	// Reuse kubectl global flags to provide namespace, context and credential options.
	// We are not using NewConfigFlags here to avoid adding too many flags
	flags := &genericclioptions.ConfigFlags{
		KubeConfig:  awsSdk.String(""),
		ClusterName: awsSdk.String(""),
		Context:     awsSdk.String(""),
		APIServer:   awsSdk.String(""),
		Timeout:     awsSdk.String("0"),
		Insecure:    awsSdk.Bool(false),
		Impersonate: awsSdk.String(""),
	}
	flags.AddFlags(cmd.PersistentFlags())
	return flags
}
