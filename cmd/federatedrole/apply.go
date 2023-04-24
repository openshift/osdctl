package federatedrole

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// newCmdApply implements the apply command to apply federated role CR
func newCmdApply(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newApplyOptions(streams, flags, client)
	applyCmd := &cobra.Command{
		Use:               "apply",
		Short:             "Apply federated role CR",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.run())
		},
		Deprecated: "This command's functionality has been added to aws-account-operator. If you are running this from an SOP, please remove that step from the SOP.",
	}

	applyCmd.Flags().StringVarP(&ops.url, "url", "u", "", "The URL of federated role yaml file")
	applyCmd.Flags().StringVarP(&ops.file, "file", "f", "", "The path of federated role yaml file")
	applyCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")

	return applyCmd
}

// applyOptions defines the struct for running list account command
type applyOptions struct {
	url  string
	file string

	verbose bool

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newApplyOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *applyOptions {
	return &applyOptions{
		flags:     flags,
		IOStreams: streams,
		kubeCli:   client,
	}
}

func (o *applyOptions) run() error {
	return nil
}
