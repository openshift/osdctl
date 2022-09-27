package account

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	k8spkg "github.com/openshift/osdctl/pkg/k8s"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
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
	consoleCmd.Flags().BoolVar(&ops.launch, "launch", false, "Launch web browser directly")

	return consoleCmd
}

// consoleOptions defines the struct for running Console command
type consoleOptions struct {
	k8sclusterresourcefactory k8spkg.ClusterResourceFactoryOptions

	verbose bool
	launch  bool

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

	awsClient, err := awsprovider.NewAwsClient(o.k8sclusterresourcefactory.Awscloudfactory.Profile, o.k8sclusterresourcefactory.Awscloudfactory.Region, o.k8sclusterresourcefactory.Awscloudfactory.ConfigFile)
	if err != nil {
		return err
	}

	partition, err := awsprovider.GetAwsPartition(awsClient)
	if err != nil {
		return err
	}

	callerIdentityOutput, err := awsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		klog.Error("Fail to get caller identity. Could you please validate the credentials?")
		return err
	}
	o.k8sclusterresourcefactory.Awscloudfactory.CallerIdentity = callerIdentityOutput
	roleArn, err := arn.Parse(aws.StringValue(callerIdentityOutput.Arn))
	if err != nil {
		return err
	}

	splitArn := strings.Split(roleArn.Resource, "/")
	username := splitArn[1]
	o.k8sclusterresourcefactory.Awscloudfactory.SessionName = fmt.Sprintf("RH-SRE-%s", username)

	o.k8sclusterresourcefactory.Awscloudfactory.RoleName = awsv1alpha1.AccountOperatorIAMRole

	consoleURL, err := awsprovider.RequestSignInToken(
		awsClient,
		&o.k8sclusterresourcefactory.Awscloudfactory.ConsoleDuration,
		aws.String(o.k8sclusterresourcefactory.Awscloudfactory.SessionName),
		aws.String(fmt.Sprintf("arn:%s:iam::%s:role/%s", partition, o.k8sclusterresourcefactory.AccountID, o.k8sclusterresourcefactory.Awscloudfactory.RoleName)),
	)
	if err != nil {
		fmt.Fprintf(o.IOStreams.Out, "Generating console failed. If CCS cluster, customer removed or denied access to the ManagedOpenShiftSupport role.")
		return err
	}

	consoleURL, err = PrependRegionToURL(consoleURL, o.k8sclusterresourcefactory.Awscloudfactory.Region)
	if err != nil {
		return fmt.Errorf("could not prepend region to console url: %w", err)
	}
	fmt.Fprintf(o.IOStreams.Out, "The AWS Console URL is:\n%s\n", consoleURL)

	if o.launch {
		return browser.OpenURL(consoleURL)
	}

	return nil
}

func PrependRegionToURL(consoleURL, region string) (string, error) {
	// Extract the url data
	u, err := url.Parse(consoleURL)
	if err != nil {
		return "", fmt.Errorf("cannot parse consoleURL '%s' : %w", consoleURL, err)
	}
	urlValues, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", fmt.Errorf("cannot parse the queries '%s' : %w", u.RawQuery, err)
	}

	// Retrieve the Destination url for modification
	rawDestinationUrl := urlValues.Get("Destination")
	destinationURL, err := url.Parse(rawDestinationUrl)
	if err != nil {
		return "", fmt.Errorf("cannot parse rawDestinationUrl '%s' : %w", rawDestinationUrl, err)
	}
	// Prepend the region to the url
	destinationURL.Host = fmt.Sprintf("%s.%s", region, destinationURL.Host)
	prependedDestinationURL := destinationURL.String()

	// override the Destination after it was modified
	urlValues.Set("Destination", prependedDestinationURL)

	// Wrap up the values into the original url
	u.RawQuery = urlValues.Encode()
	consoleURL = u.String()

	return consoleURL, nil
}
