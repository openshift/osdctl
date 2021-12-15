package account

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/spf13/cobra"

	"github.com/openshift/osdctl/internal/utils/globalflags"
	k8spkg "github.com/openshift/osdctl/pkg/k8s"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// newCmdCli implements the Cli command which generates temporary STS cli credentials for the specified account cr
func newCmdCli(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newCliOptions(streams, flags, globalOpts)
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

	cliCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")

	return cliCmd
}

// cliOptions defines the struct for running the cli command
type cliOptions struct {
	k8sclusterresourcefactory k8spkg.ClusterResourceFactoryOptions

	output  string
	verbose bool

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

func newCliOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cliOptions {
	return &cliOptions{
		k8sclusterresourcefactory: k8spkg.ClusterResourceFactoryOptions{
			Flags: flags,
		},
		IOStreams:     streams,
		GlobalOptions: globalOpts,
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

	o.output = o.GlobalOptions.Output

	return nil
}

func (o *cliOptions) run() error {
	awsClient, err := o.k8sclusterresourcefactory.GetCloudProvider(o.verbose)
	if err != nil {

		return err
	}

	creds := o.k8sclusterresourcefactory.Awscloudfactory.Credentials

	if o.k8sclusterresourcefactory.Awscloudfactory.RoleName != "OrganizationAccountAccessRole" {
		creds, err = awsprovider.GetAssumeRoleCredentials(awsClient,
			&o.k8sclusterresourcefactory.Awscloudfactory.ConsoleDuration, aws.String(o.k8sclusterresourcefactory.Awscloudfactory.SessionName),
			aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s",
				o.k8sclusterresourcefactory.AccountID,
				o.k8sclusterresourcefactory.Awscloudfactory.RoleName)))
		if err != nil {
			klog.Error("Failed to assume ManagedOpenShiftSupport role. Customer either deleted role or denied SREP access")
			return err
		}
	}

	if o.output == "" {
		fmt.Fprintf(o.IOStreams.Out, "Temporary AWS Credentials:\n%s\n", creds)
	}

	if o.output == "json" {
		fmt.Fprintf(o.IOStreams.Out, "{\n\"AccessKeyId\": %q, \n\"Expiration\": %q, \n\"SecretAccessKey\": %q, \n\"SessionToken\": %q\n}",
			*creds.AccessKeyId,
			*creds.Expiration,
			*creds.SecretAccessKey,
			*creds.SessionToken,
		)
	}

	if o.output == "env" {
		fmt.Fprintf(o.IOStreams.Out, "AWS_ACCESS_KEY_ID=%s \nAWS_SECRET_ACCESS_KEY=%s \nAWS_SESSION_TOKEN=%s \nAWS_DEFAULT_REGION=%s\n",
			*creds.AccessKeyId,
			*creds.SecretAccessKey,
			*creds.SessionToken,
			o.k8sclusterresourcefactory.Awscloudfactory.Region,
		)
	}

	return nil
}
