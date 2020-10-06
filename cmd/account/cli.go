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

	cliCmd.Flags().BoolVarP(&ops.verbose, "verbose", "v", false, "Verbose output")

	return cliCmd
}

// cliOptions defines the struct for running the cli command
type cliOptions struct {
	k8sclusterresourcefactory k8spkg.ClusterResourceFactoryOptions

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
	awsClient, err := o.k8sclusterresourcefactory.GetCloudProvider(o.verbose)
	if err != nil {
		return err
	}

	credentials, err := awsprovider.GetAssumeRoleCredentials(awsClient, &o.k8sclusterresourcefactory.Awscloudfactory.ConsoleDuration,
		o.k8sclusterresourcefactory.Awscloudfactory.CallerIdentity.UserId,
		aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s",
			o.k8sclusterresourcefactory.AccountID,
			o.k8sclusterresourcefactory.Awscloudfactory.RoleName)))
	if err != nil {
		return err
	}
	fmt.Fprintf(o.IOStreams.Out, "Temporary AWS Credentials:\n%s\n", credentials)

	return nil
}
