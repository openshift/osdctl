package account

import (
	"fmt"
	"net/url"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// newCmdConsole implements the Console command which Consoles the specified account cr
func newCmdConsole() *cobra.Command {
	ops := newConsoleOptions()
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

	consoleCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")
	consoleCmd.Flags().BoolVar(&ops.launch, "launch", false, "Launch web browser directly")
	consoleCmd.Flags().Int32VarP(&ops.consoleDuration, "duration", "d", 3600, "The duration of the console session. "+
		"Default value is 3600 seconds(1 hour)")
	consoleCmd.Flags().StringVarP(&ops.awsAccountID, "accountId", "i", "", "AWS Account ID")
	consoleCmd.Flags().StringVarP(&ops.awsProfile, "profile", "p", "", "AWS Profile")
	consoleCmd.Flags().StringVarP(&ops.region, "region", "r", "", "Region")

	return consoleCmd
}

// consoleOptions defines the struct for running Console command
type consoleOptions struct {
	verbose bool
	launch  bool

	awsAccountID string
	awsProfile   string
	region       string

	consoleDuration int32
}

func newConsoleOptions() *consoleOptions {
	return &consoleOptions{}
}

func (o *consoleOptions) complete(cmd *cobra.Command) error {

	if o.awsAccountID == "" {
		return fmt.Errorf("please specify -i")
	}

	if o.region == "" {
		o.region = "us-east-1"
	}

	return nil
}

func (o *consoleOptions) run() error {

	var err error

	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocmClient.Close()

	// Build the base AWS client using the provide credentials (profile or env vars)
	awsClient, err := aws.NewAwsClient(o.awsProfile, o.region, "")
	if err != nil {
		fmt.Printf("Could not build AWS Client: %s\n", err)
		return err
	}

	// Get the right partition for the final ARN
	partition, err := aws.GetAwsPartition(awsClient)
	if err != nil {
		return err
	}

	// Generate a session name using the SRE's kerberos ID
	sessionName, err := osdCloud.GenerateRoleSessionName(awsClient)
	if err != nil {
		fmt.Printf("Could not generate Session Name: %s\n", err)
		return err
	}

	// By default, the target role arn is OrganizationAccountAccessRole (works for -i and non-CCS clusters)
	targetRoleArnString := aws.GenerateRoleARN(o.awsAccountID, osdCloud.OrganizationAccountAccessRole)

	targetRoleArn, err := arn.Parse(targetRoleArnString)
	if err != nil {
		return err
	}

	targetRoleArn.Partition = partition

	consoleURL, err := aws.RequestSignInToken(
		awsClient,
		&o.consoleDuration,
		&sessionName,
		awsSdk.String(targetRoleArn.String()),
	)
	if err != nil {
		fmt.Printf("Generating console failed: %s\n", err)
		return err
	}

	consoleURL, err = PrependRegionToURL(consoleURL, o.region)
	if err != nil {
		return fmt.Errorf("could not prepend region to console url: %w", err)
	}
	fmt.Printf("The AWS Console URL is:\n%s\n", consoleURL)

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
