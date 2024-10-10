package support

import (
	"fmt"
	"os"
	"strconv"

	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/printer"
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
func newCmdstatus(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newStatusOptions(streams, globalOpts)
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

func newStatusOptions(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *statusOptions {
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
	clusterLimitedSupportReasons, err := getLimitedSupportReasons(o.clusterID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get limited support reasons: %v\n", err)
		return err
	}

	var limitedSupportOverridden = false
	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"Reason ID", "Summary", "Overridden (SUPPORTEX)", "Details"})
	for _, clusterLimitedSupportReason := range clusterLimitedSupportReasons {
		limitedSupportOverridden = limitedSupportOverridden || clusterLimitedSupportReason.Override().Enabled()
		table.AddRow([]string{
			clusterLimitedSupportReason.ID(),
			clusterLimitedSupportReason.Summary(),
			strconv.FormatBool(clusterLimitedSupportReason.Override().Enabled()),
			clusterLimitedSupportReason.Details(),
		})
	}
	// No reasons found, cluster is fully supported
	if limitedSupportOverridden {
		fmt.Printf("No limited support reasons found or all reasons are overridden, the cluster is fully supported\n")
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
