package main

import (
	"fmt"
	"os"

	"github.com/openshift/osdctl/cmd"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {

	err := osdctlConfig.EnsureConfigFile()
	if err != nil {
		fmt.Println(err)
		return
	}

	cobra.EnableTraverseRunHooks = true
	command := cmd.NewCmdRoot(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})

	resolved, err := command.ExecuteC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run '%s --help' for usage.\n", resolved.CommandPath())
		os.Exit(1)
	}
}
