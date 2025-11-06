package reports

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/pkg/backplane"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

type getOptions struct {
	clusterID string
	reportID  string
	output    string
}

func newCmdGet() *cobra.Command {
	opts := &getOptions{}

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get a specific cluster report from backplane-api",
		Long: `Retrieve and display a specific report by its ID.

This command fetches a report by its report ID and displays the decoded
report data. Use 'list' to find available report IDs.

Examples:
  # Get a specific report and display its contents
  osdctl cluster reports get --cluster-id 1a2b3c4d --report-id abc-123-def

  # Get a report with JSON output (includes encoded data)
  osdctl cluster reports get -C my-cluster -r report-456 --output json

  # Get a report and pipe the output to a file
  osdctl cluster reports get -C 1a2b3c4d -r abc-123 > report-output.txt`,
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

	getCmd.Flags().StringVarP(&opts.clusterID, "cluster-id", "C", "", "Cluster ID (internal or external)")
	getCmd.Flags().StringVarP(&opts.reportID, "report-id", "r", "", "Report ID to retrieve")
	getCmd.Flags().StringVarP(&opts.output, "output", "o", "text", "Output format: text or json")
	_ = getCmd.MarkFlagRequired("cluster-id")
	_ = getCmd.MarkFlagRequired("report-id")

	return getCmd
}

func (o *getOptions) run(ocmClient *sdk.Connection) error {
	// Convert external cluster ID to internal if needed
	internalClusterID, err := utils.GetInternalClusterID(ocmClient, o.clusterID)
	if err != nil {
		return err
	}

	backplaneClient, err := backplane.NewClient(internalClusterID)
	if err != nil {
		return fmt.Errorf("failed to create backplane client: %w", err)
	}

	// Fetch the specific report
	ctx := context.Background()
	report, err := backplaneClient.GetReport(ctx, o.reportID)
	if err != nil {
		return fmt.Errorf("failed to get report: %w", err)
	}

	if o.output == "json" {
		bytes, err := json.Marshal(report)
		if err != nil {
			return fmt.Errorf("failed to marshal report: %w", err)
		}
		fmt.Println(string(bytes))
		return nil
	}

	decodedData, err := base64.StdEncoding.DecodeString(report.Data)
	if err != nil {
		return fmt.Errorf("failed to decode report data: %w", err)
	}

	fmt.Printf("📒Report Details for Report %s created at %s\n\n", report.ReportId, report.CreatedAt.Format(time.RFC3339))
	fmt.Println(string(decodedData))

	return nil
}
