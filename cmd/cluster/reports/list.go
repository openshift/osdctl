package reports

import (
	"context"
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

type listOptions struct {
	clusterID string
	last      int
	output    string
}

func newCmdList() *cobra.Command {
	opts := &listOptions{}

	listCmd := &cobra.Command{
		Use:               "list",
		Short:             "List cluster reports from backplane-api",
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

	listCmd.Flags().StringVarP(&opts.clusterID, "cluster-id", "C", "", "Cluster ID (internal or external)")
	listCmd.Flags().IntVarP(&opts.last, "last", "l", 0, "Number of most recent reports to retrieve (backend defaults to 10)")
	listCmd.Flags().StringVarP(&opts.output, "output", "o", "table", "Output format: table or json")
	_ = listCmd.MarkFlagRequired("cluster-id")

	return listCmd
}

func (o *listOptions) run(ocmClient *sdk.Connection) error {
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

	reports, err := backplaneClient.ListReports(ctx, o.last)
	if err != nil {
		return fmt.Errorf("failed to list reports: %w", err)
	}

	if o.output == "json" {
		bytes, err := json.Marshal(reports)
		if err != nil {
			return fmt.Errorf("failed to marshal report: %w", err)
		}
		fmt.Println(string(bytes))
		return nil
	}

	if reports == nil || len(reports.Reports) == 0 {
		fmt.Println("No reports found")
		return nil
	}

	fmt.Printf("Found %d reports for cluster %s\n\n", len(reports.Reports), o.clusterID)

	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"Report ID", "Summary", "Created At"})
	for _, report := range reports.Reports {
		timeString := report.CreatedAt.Format(time.RFC3339)

		table.AddRow([]string{
			*report.ReportId,
			*report.Summary,
			timeString,
		})
	}

	if err := table.Flush(); err != nil {
		return fmt.Errorf("failed to print table: %w", err)
	}

	return nil
}
