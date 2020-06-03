package cmd

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/util/templates"
)

// newCmdOptions implements the options command which shows all global options
func newCmdOptions(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "options",
		Short: "Print the list of flags inherited by all commands",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	cmd.SetOut(streams.Out)
	cmd.SetErr(streams.ErrOut)

	templates.UseOptionsTemplates(cmd)
	return cmd
}
