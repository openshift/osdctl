package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// newCmdDocs implements the options command which shows all global options
func newCmdDocs(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "docs <dir>",
		Short:                 "Generates documentation files",
		Hidden:                true,
		DisableAutoGenTag:     true,
		DisableFlagsInUseLine: true,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				cmdutil.CheckErr(cmdutil.UsageErrorf(cmd, "directory name is needed"))
			}
			cmdutil.CheckErr(doc.GenMarkdownTree(cmd.Root(), args[0]))
			fmt.Fprintln(streams.Out, "Documents generated successfully on", args[0])
		},
	}

	return cmd
}
