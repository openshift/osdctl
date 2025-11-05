package reports

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/pkg/backplane"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

type createOptions struct {
	clusterID string
	summary   string
	data      string
	file      string
	output    string
}

func newCmdCreate() *cobra.Command {
	opts := &createOptions{}

	createCmd := &cobra.Command{
		Use:               "create",
		Short:             "Create a new cluster report in backplane-api",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ocmClient, err := utils.CreateConnection()
			if err != nil {
				return err
			}
			defer ocmClient.Close()

			return opts.run(ocmClient)
		},
	}

	createCmd.Flags().StringVarP(&opts.clusterID, "cluster-id", "C", "", "Cluster ID (internal or external)")
	createCmd.Flags().StringVar(&opts.summary, "summary", "", "Summary/title for the report")
	createCmd.Flags().StringVarP(&opts.data, "data", "d", "", "Report data as a string (will be base64 encoded)")
	createCmd.Flags().StringVarP(&opts.file, "file", "f", "", "Path to file containing report data (will be base64 encoded)")
	createCmd.Flags().StringVarP(&opts.output, "output", "o", "table", "Output format: table or json")

	_ = createCmd.MarkFlagRequired("cluster-id")
	_ = createCmd.MarkFlagRequired("summary")

	return createCmd
}

func (o *createOptions) run(ocmClient *sdk.Connection) error {
	// Validate that either data or file is provided, but not both
	if o.data == "" && o.file == "" {
		return fmt.Errorf("either --data or --file must be provided")
	}
	if o.data != "" && o.file != "" {
		return fmt.Errorf("cannot specify both --data and --file")
	}

	// Read data from file if provided
	var rawData []byte
	if o.file != "" {
		var err error
		rawData, err = os.ReadFile(o.file)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
	} else {
		rawData = []byte(o.data)
	}

	// Encode data to base64
	encodedData := base64.StdEncoding.EncodeToString(rawData)

	// Convert external cluster ID to internal if needed
	internalClusterID, err := utils.GetInternalClusterID(ocmClient, o.clusterID)
	if err != nil {
		return err
	}

	// Create backplane client
	backplaneClient, err := backplane.NewClient(internalClusterID)
	if err != nil {
		return fmt.Errorf("failed to create backplane client: %w", err)
	}

	// Create the report
	ctx := context.Background()
	report, err := backplaneClient.CreateReport(ctx, o.summary, encodedData)
	if err != nil {
		return fmt.Errorf("failed to create report: %w", err)
	}

	// Display results
	if o.output == "json" {
		bytes, err := json.Marshal(report)
		if err != nil {
			return fmt.Errorf("failed to marshal report: %w", err)
		}
		fmt.Println(string(bytes))
		return nil
	}

	// Print as table
	fmt.Printf("Successfully created report for cluster %s\n\n", o.clusterID)

	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"Field", "Value"})
	table.AddRow([]string{"Report ID", report.ReportId})
	table.AddRow([]string{"Summary", report.Summary})
	table.AddRow([]string{"Created At", report.CreatedAt.Format(time.RFC3339)})

	if err := table.Flush(); err != nil {
		return fmt.Errorf("failed to print table: %w", err)
	}

	return nil
}
