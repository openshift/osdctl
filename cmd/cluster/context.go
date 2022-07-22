package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/cmd/servicelog"
	sl "github.com/openshift/osdctl/internal/servicelog"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/config"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type statusOptions struct {
	output     string
	verbose    bool
	clusterID  string
	baseDomain string
	days       int
	oauthtoken string

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

type limitedSupportReasonItem struct {
	ID      string
	Summary string
	Details string
}

// newCmdContext implements the context command to show the current context of a cluster
func newCmdContext(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newStatusOptions(streams, flags, globalOpts)
	contextCmd := &cobra.Command{
		Use:               "context",
		Short:             "Shows the context of a specified cluster",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}
	contextCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")
	contextCmd.Flags().IntVarP(&ops.days, "days", "d", 30, "Command will display X days of Error SLs sent to the cluster. Days is set to 30 by default")
	contextCmd.Flags().StringVarP(&ops.oauthtoken, "oauthtoken", "t", "", "Pass in PD oauthtoken directly. If not passed in, by default will read token from ~/.config/pagerduty-cli/config.json")

	return contextCmd
}

func newStatusOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *statusOptions {
	return &statusOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

func (o *statusOptions) complete(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "Provide exactly one cluster ID")
	}

	if o.days < 1 {
		return fmt.Errorf("Cannot have a days value lower than 1")
	}

	// Create OCM client to talk to cluster API
	ocmClient := utils.CreateConnection()
	defer func() {
		if err := ocmClient.Close(); err != nil {
			fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	clusters := utils.GetClusters(ocmClient, args)
	if len(clusters) != 1 {
		return fmt.Errorf("unexpected number of clusters matched input. Expected 1 got %d", len(clusters))
	}
	o.clusterID = clusters[0].ID()
	o.baseDomain = clusters[0].DNS().BaseDomain()
	o.output = o.GlobalOptions.Output

	return nil
}

func (o *statusOptions) run() error {
	// Create a context:
	ctx := context.Background()

	// Ocm token
	token := getOCMToken()
	connection, err := sdk.NewConnectionBuilder().
		Tokens(token).
		Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't build connection: %v\n", err)
		os.Exit(1)
	}
	defer connection.Close()

	// Get limited support reasons for a cluster
	lsResponse, err := getLimitedSupportReasons(connection, ctx, o.clusterID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't retrieve cluster limited support reasons: %v\n", err)
		os.Exit(1)
	}

	// Check support status of cluster
	err = printSupportStatus(lsResponse)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't print support status: %v\n", err)
		os.Exit(1)
	}

	// Print the Servicelogs for this cluster
	err = o.printServiceLogs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't print service logs: %v\n", err)
		os.Exit(1)
	}

	// Print all triggered and acknowledged pd alerts
	err = o.printPDAlerts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't print pagerduty alerts: %v\n", err)
		os.Exit(1)
	}

	return nil
}

func getOCMToken() string {
	token := os.Getenv("OCM_TOKEN")
	if token == "" {
		ocmToken, err := utils.GetOCMAccessToken()
		if err != nil {
			log.Fatalf("OCM token not set. Please configure by using the OCM_TOKEN environment variable or the ocm cli")
			os.Exit(1)
		}
		token = *ocmToken
	}
	return token
}

func getLimitedSupportReasons(connection *sdk.Connection, ctx context.Context, clusterID string) (*cmv1.LimitedSupportReasonsListResponse, error) {
	collection := connection.ClustersMgmt().V1().Clusters()
	resource := collection.Cluster(clusterID).LimitedSupportReasons()
	lsResponse, err := resource.List().SendContext(ctx)
	return lsResponse, err
}

// printSupportStatus reports if a cluster is in limited support or fully supported.
func printSupportStatus(response *cmv1.LimitedSupportReasonsListResponse) error {
	reasons, _ := response.GetItems()
	var clusterLimitedSupportReasons []*limitedSupportReasonItem
	reasons.Each(func(limitedSupportReason *cmv1.LimitedSupportReason) bool {
		clusterLimitedSupportReason := limitedSupportReasonItem{
			ID:      limitedSupportReason.ID(),
			Summary: limitedSupportReason.Summary(),
			Details: limitedSupportReason.Details(),
		}
		clusterLimitedSupportReasons = append(clusterLimitedSupportReasons, &clusterLimitedSupportReason)
		return true
	})

	fmt.Println("============================================================")
	fmt.Println("Limited Support Status")
	fmt.Println("============================================================")

	// No reasons found, cluster is fully supported
	if len(clusterLimitedSupportReasons) == 0 {
		fmt.Printf("Cluster is fully supported\n")
		fmt.Println()
		return nil
	}

	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"Reason ID", "Summary", "Details"})
	for _, clusterLimitedSupportReason := range clusterLimitedSupportReasons {
		table.AddRow([]string{clusterLimitedSupportReason.ID, clusterLimitedSupportReason.Summary, clusterLimitedSupportReason.Details})
	}
	// Add empty row for readability
	table.AddRow([]string{})
	table.Flush()

	return nil
}

func (o *statusOptions) printServiceLogs() error {

	// Get the SLs for the cluster
	slResponse, err := servicelog.FetchServiceLogs(o.clusterID)
	if err != nil {
		return err
	}

	var serviceLogs sl.ServiceLogShortList
	err = json.Unmarshal(slResponse.Bytes(), &serviceLogs)
	if err != nil {
		fmt.Printf("Failed to unmarshal the SL response %q\n", err)
		return err
	}

	// Parsing the relevant servicelogs
	// - We only care about Error Severity SLs
	// - We only care about SLs sent in the past 'o.days' days
	var errorServiceLogs []sl.ServiceLogShort
	for _, serviceLog := range serviceLogs.Items {
		if serviceLog.Severity != "Error" {
			continue
		}

		// If the days since the SL was sent exceeds o.days days, we're not interested
		if (time.Since(serviceLog.CreatedAt).Hours() / 24) > float64(o.days) {
			continue
		}

		errorServiceLogs = append(errorServiceLogs, serviceLog)
	}

	fmt.Println("============================================================")
	fmt.Println("Service Logs with Error Severity sent in the past", o.days, "Days")
	fmt.Println("============================================================")

	if o.verbose {
		marshalledSLs, err := json.MarshalIndent(errorServiceLogs, "", "  ")
		if err != nil {
			return err
		}
		dump.Pretty(os.Stdout, marshalledSLs)
	} else {
		// Non verbose only prints the summaries
		for i, errorServiceLog := range errorServiceLogs {
			fmt.Printf("%d. %s \n", i, errorServiceLog.Summary)
		}
	}
	fmt.Println()

	return nil
}

func (o *statusOptions) printPDAlerts() error {
	var oauthtoken string
	if o.oauthtoken != "" {
		oauthtoken = o.oauthtoken
	} else {
		pdConfig := config.LoadPDConfig("/.config/pagerduty-cli/config.json")
		oauthtoken = pdConfig.MySubdomain[0].AccessToken
	}
	client := pd.NewOAuthClient(oauthtoken)

	ctx := context.TODO()
	lsResponse, err := client.ListServicesWithContext(ctx, pd.ListServiceOptions{Query: o.baseDomain})

	if err != nil {
		fmt.Printf("Failed to ListServicesWithContext %q\n", err)
		return err
	}

	if len(lsResponse.Services) != 1 {
		return fmt.Errorf("unexpected number of services matched input. Expected 1 got %d", len(lsResponse.Services))
	}

	serviceID := lsResponse.Services[0].ID
	liResponse, err := client.ListIncidentsWithContext(
		ctx,
		pd.ListIncidentsOptions{
			ServiceIDs: []string{serviceID},
			Statuses:   []string{"triggered", "acknowledged"},
		},
	)
	if err != nil {
		fmt.Printf("Failed to ListIncidentsWithContext %q\n", err)
		return err
	}

	fmt.Println("============================================================")
	fmt.Println("Pagerduty alerts for the Cluster")
	fmt.Println("============================================================")
	fmt.Printf("Link to PD Service: https://redhat.pagerduty.com/service-directory/%s\n", serviceID)
	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"Urgency", "Title", "Created At"})
	for _, incident := range liResponse.Incidents {
		table.AddRow([]string{incident.Urgency, incident.Title, incident.CreatedAt})
	}
	// Add empty row for readability
	table.AddRow([]string{})
	err = table.Flush()
	if err != nil {
		fmt.Println("error while flushing table: ", err.Error())
		return err
	}

	return err
}
