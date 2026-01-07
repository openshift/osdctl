package cost

import (
	"fmt"
	"os"
	"regexp"

	"github.com/openshift/osdctl/internal/utils/globalflags"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const (
	expectedAccountName = "rh-control"
)

// carbonReportOptions defines the struct for running carbon-report command
type carbonReportOptions struct {
	usagePeriod string

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

func newCarbonReportOptions(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *carbonReportOptions {
	return &carbonReportOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

// validateUsagePeriod validates that the usage period is in YYYY or YYYY-MM format
func (o *carbonReportOptions) validateUsagePeriod() error {
	if o.usagePeriod == "" {
		return fmt.Errorf("usage period is required")
	}

	// Regex for YYYY format (year only)
	yearRegex := regexp.MustCompile(`^\d{4}$`)
	// Regex for YYYY-MM format (year and month)
	yearMonthRegex := regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

	if yearRegex.MatchString(o.usagePeriod) {
		return nil
	}

	if yearMonthRegex.MatchString(o.usagePeriod) {
		return nil
	}

	return fmt.Errorf("invalid usage period format '%s'. Expected format: YYYY or YYYY-MM", o.usagePeriod)
}

// CreateAWSClient creates an AWS client after validating the AWS_ACCOUNT_NAME environment variable
func CreateAWSClient() (awsprovider.Client, error) {
	// Check for AWS_ACCOUNT_NAME environment variable
	awsAccountName := os.Getenv("AWS_ACCOUNT_NAME")
	if awsAccountName != expectedAccountName {
		if awsAccountName == "" {
			return nil, fmt.Errorf("AWS_ACCOUNT_NAME environment variable is not set. Please run 'rh-aws-saml-login rh-control' first")
		}
		return nil, fmt.Errorf("AWS_ACCOUNT_NAME is set to '%s' but expected '%s'. Please run 'rh-aws-saml-login rh-control' first", awsAccountName, expectedAccountName)
	}

	// Initialize AWS client (which includes S3 client)
	awsClient, err := opsCost.initAWSClients()
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS client. Please run 'rh-aws-saml-login rh-control' first: %w", err)
	}

	// Verify we can access AWS by getting caller identity
	_, err = awsClient.GetCallerIdentity(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to verify AWS credentials. Please run 'rh-aws-saml-login rh-control' first: %w", err)
	}

	return awsClient, nil
}

// newCmdCarbonReport represents the carbon-report command
func newCmdCarbonReport(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newCarbonReportOptions(streams, globalOpts)
	carbonReportCmd := &cobra.Command{
		Use:   "carbon-report",
		Short: "Generate carbon emissions report",
		Run: func(cmd *cobra.Command, args []string) {
			// Validate usage period
			cmdutil.CheckErr(ops.validateUsagePeriod())

			awsClient, err := CreateAWSClient()
			cmdutil.CheckErr(err)

			// Use awsClient for S3 operations
			_ = awsClient

			fmt.Printf("Hello World - Usage Period: %s\n", ops.usagePeriod)
		},
	}

	carbonReportCmd.Flags().StringVar(&ops.usagePeriod, "usage-period", "", "Usage period in YYYY or YYYY-MM format")

	return carbonReportCmd
}
