package cost

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const (
	expectedAccountName = "rh-control"
)

var (
	// excludedColumns defines columns that should not be exported in the final CSV
	excludedColumns = map[string]bool{
		"payer_account_id": true,
		"usage_account_id": true,
	}
)

// carbonReportOptions defines the struct for running carbon-report command
type carbonReportOptions struct {
	usagePeriod string
	account     string

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

// validateAccount validates that the account is a number with at least 12 digits
func (o *carbonReportOptions) validateAccount() error {
	if o.account == "" {
		return fmt.Errorf("account is required")
	}

	// Regex for a number with at least 12 digits
	accountRegex := regexp.MustCompile(`^\d{12,}$`)

	if !accountRegex.MatchString(o.account) {
		return fmt.Errorf("invalid account format '%s'. Account must be a number with at least 12 digits", o.account)
	}

	return nil
}

// CreateAWSClient creates an AWS client after validating the AWS_ACCOUNT_NAME environment variable
func CreateAWSClient() (awsprovider.Client, error) {
	return createAWSClientWithOptions(opsCost)
}

// createAWSClientWithOptions creates an AWS client with the provided costOptions
// This function is separated for testing purposes
func createAWSClientWithOptions(opts *costOptions) (awsprovider.Client, error) {
	// Check for AWS_ACCOUNT_NAME environment variable
	awsAccountName := os.Getenv("AWS_ACCOUNT_NAME")
	if awsAccountName != expectedAccountName {
		if awsAccountName == "" {
			return nil, fmt.Errorf("AWS_ACCOUNT_NAME environment variable is not set. Please run 'rh-aws-saml-login rh-control' first")
		}
		return nil, fmt.Errorf("AWS_ACCOUNT_NAME is set to '%s' but expected '%s'. Please run 'rh-aws-saml-login rh-control' first", awsAccountName, expectedAccountName)
	}

	// Initialize AWS client (which includes S3 client)
	awsClient, err := opts.initAWSClients()
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

// getUsagePeriodDirectories retrieves S3 directories matching the usage period pattern
func getUsagePeriodDirectories(awsClient awsprovider.Client, usagePeriod string) ([]string, error) {
	bucketName := "rhcontrol-ccft-reports"
	basePath := "reports/carbon-emissions/data/carbon_model_version=v3.0.0/"
	delimiter := "/"

	result, err := awsClient.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket:    aws.String(bucketName),
		Prefix:    aws.String(basePath),
		Delimiter: aws.String(delimiter),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list S3 objects: %w", err)
	}

	var matchingDirs []string
	usagePeriodPrefix := "usage_period="

	// Extract year and optional month from usage period
	var yearFilter, monthFilter string
	if len(usagePeriod) == 4 {
		// YYYY format - match all months in this year
		yearFilter = usagePeriod
	} else {
		// YYYY-MM format - match specific month
		yearFilter = usagePeriod[:4]
		monthFilter = usagePeriod[5:7]
	}

	// Check CommonPrefixes for matching directories
	for _, prefix := range result.CommonPrefixes {
		if prefix.Prefix == nil {
			continue
		}

		// Extract the directory name from the full prefix
		// Format: reports/carbon-emissions/data/carbon_model_version=v3.0.0/usage_period=YYYY-MM/
		prefixStr := *prefix.Prefix

		// Find the usage_period part
		if idx := regexp.MustCompile(`usage_period=(\d{4}-\d{2})/`).FindStringSubmatch(prefixStr); len(idx) > 1 {
			dirPeriod := idx[1] // e.g., "2024-03"
			dirYear := dirPeriod[:4]
			dirMonth := dirPeriod[5:7]

			// Check if it matches our filter
			if dirYear == yearFilter {
				if monthFilter == "" || dirMonth == monthFilter {
					matchingDirs = append(matchingDirs, usagePeriodPrefix+dirPeriod)
				}
			}
		}
	}

	return matchingDirs, nil
}

// processCarbonData downloads and processes carbon emissions data for a specific usage period directory
func processCarbonData(awsClient awsprovider.Client, bucketName, usagePeriodDir, accountID string) ([][]string, []string, error) {
	basePath := "reports/carbon-emissions/data/carbon_model_version=v3.0.0/"
	prefix := basePath + usagePeriodDir + "/"

	// List objects in the directory to find the .gz file
	result, err := awsClient.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list objects in %s: %w", usagePeriodDir, err)
	}

	// Find the .gz file
	var gzFile string
	for _, obj := range result.Contents {
		if obj.Key != nil && strings.HasSuffix(*obj.Key, ".gz") {
			gzFile = *obj.Key
			break
		}
	}

	if gzFile == "" {
		return nil, nil, fmt.Errorf("no .gz file found in %s", usagePeriodDir)
	}

	// Download the .gz file
	getResult, err := awsClient.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(gzFile),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to download %s: %w", gzFile, err)
	}
	defer getResult.Body.Close()

	// Decompress the gzip file
	gzReader, err := gzip.NewReader(getResult.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decompress %s: %w", gzFile, err)
	}
	defer gzReader.Close()

	// Parse the CSV
	csvReader := csv.NewReader(gzReader)

	// Read the header
	header, err := csvReader.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Find the usage_account_id column index and identify columns to exclude
	accountIDCol := -1
	var excludeIndices []int
	var filteredHeader []string

	for i, col := range header {
		if col == "usage_account_id" {
			accountIDCol = i
		}
		// Check if this column should be excluded
		if excludedColumns[col] {
			excludeIndices = append(excludeIndices, i)
		} else {
			filteredHeader = append(filteredHeader, col)
		}
	}

	if accountIDCol == -1 {
		return nil, nil, fmt.Errorf("usage_account_id column not found in CSV")
	}

	// Create a map for quick lookup of excluded indices
	excludeMap := make(map[int]bool)
	for _, idx := range excludeIndices {
		excludeMap[idx] = true
	}

	// Read and filter rows
	var filteredRows [][]string
	for {
		row, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read CSV row: %w", err)
		}

		// Filter by account ID
		if accountIDCol < len(row) && row[accountIDCol] == accountID {
			// Filter out excluded columns
			var filteredRow []string
			for i, val := range row {
				if !excludeMap[i] {
					filteredRow = append(filteredRow, val)
				}
			}
			filteredRows = append(filteredRows, filteredRow)
		}
	}

	return filteredRows, filteredHeader, nil
}

// newCmdCarbonReport represents the carbon-report command
func newCmdCarbonReport(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newCarbonReportOptions(streams, globalOpts)
	carbonReportCmd := &cobra.Command{
		Use:   "carbon-report",
		Short: "Generate carbon emissions report csv to stdout for a given AWS Account and Usage Period",
		Run: func(cmd *cobra.Command, args []string) {
			// Validate usage period
			cmdutil.CheckErr(ops.validateUsagePeriod())

			// Validate account
			cmdutil.CheckErr(ops.validateAccount())

			awsClient, err := CreateAWSClient()
			cmdutil.CheckErr(err)

			// Get usage period directories from S3
			directories, err := getUsagePeriodDirectories(awsClient, ops.usagePeriod)
			cmdutil.CheckErr(err)

			if len(directories) == 0 {
				log.Printf("No directories found for usage period: %s", ops.usagePeriod)
				return
			}

			bucketName := "rhcontrol-ccft-reports"
			var allRows [][]string
			var csvHeader []string

			// Process each directory
			for _, dir := range directories {
				log.Printf("Processing usage period: %s", dir)

				rows, header, err := processCarbonData(awsClient, bucketName, dir, ops.account)
				if err != nil {
					cmdutil.CheckErr(fmt.Errorf("error processing %s: %w", dir, err))
				}

				if len(csvHeader) == 0 {
					csvHeader = header
				}

				allRows = append(allRows, rows...)
				log.Printf("Found %d rows for account %s in %s", len(rows), ops.account, dir)
			}

			// Write CSV to stdout
			csvWriter := csv.NewWriter(os.Stdout)
			defer csvWriter.Flush()

			// Write header
			if err := csvWriter.Write(csvHeader); err != nil {
				cmdutil.CheckErr(fmt.Errorf("failed to write CSV header: %w", err))
			}

			// Write all rows
			for _, row := range allRows {
				if err := csvWriter.Write(row); err != nil {
					cmdutil.CheckErr(fmt.Errorf("failed to write CSV row: %w", err))
				}
			}

			log.Printf("Total rows exported: %d", len(allRows))
		},
	}

	carbonReportCmd.Flags().StringVar(&ops.usagePeriod, "usage-period", "", "Usage period in YYYY or YYYY-MM format")
	carbonReportCmd.Flags().StringVar(&ops.account, "account", "", "AWS account number")

	return carbonReportCmd
}
