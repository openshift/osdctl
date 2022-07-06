package cluster

import (
	"context"
	"fmt"
	"log"
	"os"

	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type statusOptions struct {
	output    string
	verbose   bool
	clusterID string

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

type limitedSupportReasonItem struct {
	ID      string
	Summary string
	Details string
}

// newCmdcontext implements the context command to show the current context of a cluster
func newCmdcontext(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newStatusOptions(streams, flags, globalOpts)
	statusCmd := &cobra.Command{
		Use:               "context",
		Short:             "Shows the context of a specified cluster",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}
	statusCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")

	return statusCmd
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
	o.output = o.GlobalOptions.Output

	return nil
}

func (o *statusOptions) run() error {
	// Create a context:
	ctx := context.Background()
	// Ocm token
	token := os.Getenv("OCM_TOKEN")
	if token == "" {
		ocmToken, err := utils.GetOCMAccessToken()
		if err != nil {
			log.Fatalf("OCM token not set. Please configure by using the OCM_TOKEN environment variable or the ocm cli")
			os.Exit(1)
		}
		token = *ocmToken
	}
	connection, err := sdk.NewConnectionBuilder().
		Tokens(token).
		Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't build connection: %v\n", err)
		os.Exit(1)
	}
	defer connection.Close()

	// Get limited support reasons for a cluster
	collection := connection.ClustersMgmt().V1().Clusters()
	resource := collection.Cluster(o.clusterID).LimitedSupportReasons()
	response, err := resource.List().SendContext(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't retrieve cluster limited support reasons: %v\n", err)
		os.Exit(1)
	}

	// Check support status of cluster
	checkSupportStatus(response)

	return nil
}

// checkSupportStatus reports if a cluster is in limited support or fully supported.
func checkSupportStatus(response *cmv1.LimitedSupportReasonsListResponse) error {
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

	// No reasons found, cluster is fully supported
	if len(clusterLimitedSupportReasons) == 0 {
		fmt.Printf("Cluster is fully supported\n")
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
