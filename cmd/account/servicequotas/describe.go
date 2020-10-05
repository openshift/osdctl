package servicequotas

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	k8spkg "github.com/openshift/osd-utils-cli/pkg/k8s"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
)

// newCmdDescribe implements servicequotas describe
func newCmdDescribe(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newDescribeOptions(streams, flags)
	describeCmd := &cobra.Command{
		Use:               "describe",
		Short:             "Describe AWS service-quotas",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
	}

	ops.k8scloudfactory.AttachCobraCliFlags(describeCmd)

	describeCmd.Flags().BoolVarP(&ops.verbose, "verbose", "v", false, "Verbose output")

	return describeCmd
}

// describeOptions defines the struct for running list account command
type describeOptions struct {
	k8scloudfactory k8spkg.ClusterResourceFactoryOptions

	verbose bool

	genericclioptions.IOStreams
}

func newDescribeOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *describeOptions {
	return &describeOptions{
		k8scloudfactory: k8spkg.ClusterResourceFactoryOptions{
			Flags: flags,
		},
		IOStreams: streams,
	}
}

func (o *describeOptions) complete(cmd *cobra.Command) error {
	k8svalid, err := o.k8scloudfactory.ValidateIdentifiers()
	if !k8svalid {
		if err != nil {
			return err
		}
	}

	awsvalid, err := o.k8scloudfactory.Awscloudfactory.ValidateIdentifiers()
	if !awsvalid {
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *describeOptions) run() error {
	awsClient, err := o.k8scloudfactory.GetCloudProvider(o.verbose)
	if err != nil {
		return err
	}

	consoleURL, err := awsprovider.RequestSignInToken(awsClient, &o.k8scloudfactory.Awscloudfactory.ConsoleDuration,
		aws.String(o.k8scloudfactory.Awscloudfactory.SessionName), aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s",
			o.k8scloudfactory.AccountID, o.k8scloudfactory.Awscloudfactory.RoleName)))
	if err != nil {
		return err
	}
	fmt.Fprintf(o.IOStreams.Out, "The AWS Console URL is:\n%s\n", consoleURL)

	return nil
}
