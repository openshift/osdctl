package support

import (
	"fmt"
	"os"

	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/printer"
	ctlutil "github.com/openshift/osdctl/pkg/utils"
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

// newCmdsupportCheck implements the supportCheck command to show the support status of a cluster
func newCmdstatus(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newStatusOptions(streams, flags, globalOpts)
	statusCmd := &cobra.Command{
		Use:               "status",
		Short:             "Shows the support status of a specified cluster",
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

	o.clusterID = args[0]
	o.output = o.GlobalOptions.Output

	return nil
}

func (o *statusOptions) run() error {

	//create connection to sdk
	connection := ctlutil.CreateConnection()
	defer func() {
		if err := connection.Close(); err != nil {
			fmt.Printf("Cannot close the connection: %q\n", err)
			os.Exit(1)
		}
	}()

	//getting the cluster
	cluster, err := ctlutil.GetCluster(connection, o.clusterID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't retrieve cluster: %v\n", err)
		os.Exit(1)
	}

	//getting the limited support reasons for the cluster
	clusterLimitedSupportReasons, err := ctlutil.GetClusterLimitedSupportReasons(connection, cluster.ID())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't retrieve cluster limited support reasons: %v\n", err)
		os.Exit(1)
	}

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
	err = table.Flush()
	if err != nil {
		fmt.Println("error while flushing table: ", err.Error())
		return err
	}

	return nil
}
