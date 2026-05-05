package globalflags

import (
	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// GlobalOptions defines all available commands
type GlobalOptions struct {
	SkipVersionCheck bool
	Output           string
	NoAwsProxy       bool
	KubeFlags        genericclioptions.ConfigFlags
}

func NewGlobalOptions() *GlobalOptions {
	return &GlobalOptions{
		KubeFlags: genericclioptions.ConfigFlags{
			KubeConfig:  awsSdk.String(""),
			ClusterName: awsSdk.String(""),
			Context:     awsSdk.String(""),
			APIServer:   awsSdk.String(""),
			Timeout:     awsSdk.String("0"),
			Insecure:    awsSdk.Bool(false),
			Impersonate: awsSdk.String("")},
	}
}

func (opts *GlobalOptions) AddSkipVersionCheckFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVarP(&opts.SkipVersionCheck, "skip-version-check", "S", false, "skip checking to see if this is the most recent release")
}

func (opts *GlobalOptions) AddOutputFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&opts.Output, "output", "o", "", "Valid formats are ['', 'json', 'yaml', 'env']")
}

func (opts *GlobalOptions) AddNoAwsProxyFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVar(&opts.NoAwsProxy, aws.NoProxyFlag, false, "Don't use the configured `aws_proxy` value")
}

func (opts *GlobalOptions) AddKubeFlags(cmd *cobra.Command) {
	opts.KubeFlags.AddFlags(cmd.PersistentFlags())
}
