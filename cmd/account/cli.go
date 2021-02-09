package account

import (
	"fmt"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	k8spkg "github.com/openshift/osd-utils-cli/pkg/k8s"
)

// newCmdCli implements the Cli command which generates temporary STS cli credentials for the specified account cr
func newCmdCli(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newCliOptions(streams, flags)
	cliCmd := &cobra.Command{
		Use:               "cli",
		Short:             "Generate temporary AWS CLI credentials on demand",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
	}

	ops.k8sclusterresourcefactory.AttachCobraCliFlags(cliCmd)

	cliCmd.Flags().StringVarP(&ops.output, "out", "o", "default", "Output format [default | json | env]")
	cliCmd.Flags().BoolVarP(&ops.verbose, "verbose", "m", false, "Verbose output")

	return cliCmd
}

// cliOptions defines the struct for running the cli command
type cliOptions struct {
	k8sclusterresourcefactory k8spkg.ClusterResourceFactoryOptions

	output  string
	verbose bool

	genericclioptions.IOStreams
}

func newCliOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cliOptions {
	return &cliOptions{
		k8sclusterresourcefactory: k8spkg.ClusterResourceFactoryOptions{
			Flags: flags,
		},
		IOStreams: streams,
	}
}

func (o *cliOptions) complete(cmd *cobra.Command) error {
	k8svalid, err := o.k8sclusterresourcefactory.ValidateIdentifiers()
	if !k8svalid {
		if err != nil {
			return err
		}
	}

	awsvalid, err := o.k8sclusterresourcefactory.Awscloudfactory.ValidateIdentifiers()
	if !awsvalid {
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *cliOptions) run() error {
	_, err := o.k8sclusterresourcefactory.GetCloudProvider(o.verbose)
	if err != nil {
		return err
	}

	creds := o.k8sclusterresourcefactory.Awscloudfactory.Credentials

	if o.output == "default" {
		fmt.Fprintf(o.IOStreams.Out, "Temporary AWS Credentials:\n%s\n", creds)
	}

	if o.output == "json" {
		fmt.Fprintf(o.IOStreams.Out, "%s\n", creds)
	}

	if o.output == "env" {
		fmt.Fprintf(o.IOStreams.Out, "AWS_ACCESS_KEY_ID=%s AWS_SECRET_ACCESS_KEY=%s AWS_SESSION_TOKEN=%s AWS_DEFAULT_REGION=%s",
			*creds.AccessKeyId,
			*creds.SecretAccessKey,
			*creds.SessionToken,
			o.k8sclusterresourcefactory.Awscloudfactory.Region,
		)
	}

	return nil
}
