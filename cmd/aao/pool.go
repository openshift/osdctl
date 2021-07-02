package aao

import (
	"fmt"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	//"k8s.io/klog"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// newCmdPool gets the current status of the AWS Account Operator AccountPool
func newCmdPool(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newPoolOptions(streams, flags)
	poolCmd := &cobra.Command{
		Use:               "pool",
		Short:             "Get the status of the AWS Account Operator AccountPool",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
	}

	return poolCmd
}

// poolOptions defines the struct for running the pool command
type poolOptions struct {
	genericclioptions.IOStreams
}

func newPoolOptions(streams genericclioptions.IOStreams, _ *genericclioptions.ConfigFlags) *poolOptions {
	return &poolOptions{
		IOStreams: streams,
	}
}

func (o *poolOptions) complete(cmd *cobra.Command) error {
	return nil
}

func (o *poolOptions) run() error {
	fmt.Println("test")
	return nil
}
