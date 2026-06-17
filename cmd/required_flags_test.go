package cmd

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestRequiredFlagsDocumentedInExamples(t *testing.T) {
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	root := NewCmdRoot(streams)

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.HasSubCommands() {
			for _, sub := range cmd.Commands() {
				walk(sub)
			}
			return
		}

		var names []string
		shorthands := map[string]string{}
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if ann, ok := f.Annotations[cobra.BashCompOneRequiredFlag]; ok && len(ann) > 0 && ann[0] == "true" {
				names = append(names, f.Name)
				if f.Shorthand != "" {
					shorthands[f.Name] = f.Shorthand
				}
			}
		})
		if len(names) == 0 {
			return
		}

		path := cmd.CommandPath()
		if cmd.Example == "" {
			t.Errorf("command %q has required flags %v but no Example field", path, names)
			return
		}
		for _, name := range names {
			if exampleContainsFlag(cmd.Example, name, shorthands[name]) {
				continue
			}
			t.Errorf("command %q Example omits required flag --%s", path, name)
		}
	}
	walk(root)
}

func exampleContainsFlag(example, name, shorthand string) bool {
	if strings.Contains(example, fmt.Sprintf("--%s", name)) {
		return true
	}
	if shorthand != "" && strings.Contains(example, fmt.Sprintf("-%s ", shorthand)) {
		return true
	}
	return false
}
