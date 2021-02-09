package account

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	k8spkg "github.com/openshift/osd-utils-cli/pkg/k8s"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
)

// newCmdConsole implements the Console command which Consoles the specified account cr
func newCmdConsole(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newConsoleOptions(streams, flags)
	consoleCmd := &cobra.Command{
		Use:               "console",
		Short:             "Generate an AWS console URL on the fly",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
	}

	ops.k8sclusterresourcefactory.AttachCobraCliFlags(consoleCmd)

	consoleCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")

	return consoleCmd
}

// consoleOptions defines the struct for running Console command
type consoleOptions struct {
	k8sclusterresourcefactory k8spkg.ClusterResourceFactoryOptions

	verbose bool

	genericclioptions.IOStreams
}

func newConsoleOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *consoleOptions {
	return &consoleOptions{
		k8sclusterresourcefactory: k8spkg.ClusterResourceFactoryOptions{
			Flags: flags,
		},
		IOStreams: streams,
	}
}

func (o *consoleOptions) complete(cmd *cobra.Command) error {
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

func (o *consoleOptions) run() error {
	awsClient, err := o.k8sclusterresourcefactory.GetCloudProvider(o.verbose)
	if err != nil {
		return err
	}

	consoleURL, err := awsprovider.RequestSignInToken(awsClient, &o.k8sclusterresourcefactory.Awscloudfactory.ConsoleDuration,
		aws.String(o.k8sclusterresourcefactory.Awscloudfactory.SessionName), aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s",
			o.k8sclusterresourcefactory.AccountID, o.k8sclusterresourcefactory.Awscloudfactory.RoleName)))
	if err != nil {
		return err
	}
	fmt.Fprintf(o.IOStreams.Out, "The AWS Console URL is:\n%s\n", consoleURL)

	return nil
}
