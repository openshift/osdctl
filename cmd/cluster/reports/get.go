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

If no report ID is provided, the latest report for the cluster is returned.`,
		Example: `  # Get a specific report
  osdctl cluster reports get --cluster-id ${CLUSTER_ID} --report-id ${REPORT_ID}

  # Get the latest report
  osdctl cluster reports get --cluster-id ${CLUSTER_ID}

  # Get a report with JSON output
  osdctl cluster reports get --cluster-id ${CLUSTER_ID} --report-id ${REPORT_ID} --output json`,
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
	getCmd.Flags().StringVarP(&opts.reportID, "report-id", "r", "", "Report ID to retrieve (defaults to the latest report if omitted)")
	getCmd.Flags().StringVarP(&opts.output, "output", "o", "text", "Output format: text or json")
	_ = getCmd.MarkFlagRequired("cluster-id")

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

	ctx := context.Background()

	reportID := o.reportID
	if reportID == "" {
		fmt.Fprintln(os.Stderr, "No report ID provided; searching for the latest report.")

		latestID, err := latestReportID(ctx, backplaneClient)
		if err != nil {
			return err
		}
		if latestID == "" {
			fmt.Fprintln(os.Stderr, "No reports found for cluster.")
			return nil
		}
		reportID = latestID
	}

	// Fetch the specific report
	report, err := backplaneClient.GetReport(ctx, reportID)
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

// latestReportID returns the report ID of the most recently created report for
// the cluster. It returns an empty string if the cluster has no reports.
func latestReportID(ctx context.Context, backplaneClient *backplane.Client) (string, error) {
	reports, err := backplaneClient.ListReports(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("failed to list reports: %w", err)
	}

	if reports == nil || len(reports.Reports) == 0 {
		return "", nil
	}

	var latestID string
	var latestTime time.Time
	for _, report := range reports.Reports {
		if report.ReportId == nil || report.CreatedAt == nil {
			continue
		}
		if latestID == "" || report.CreatedAt.After(latestTime) {
			latestID = *report.ReportId
			latestTime = *report.CreatedAt
		}
	}

	return latestID, nil
}
