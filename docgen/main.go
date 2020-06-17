package main

import (
	"fmt"
	"os"

	"github.com/openshift/osd-utils-cli/cmd"
	"github.com/spf13/cobra/doc"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s [output directory] \n", os.Args[0])
		os.Exit(1)
	}
	command := cmd.NewCmdRoot(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := doc.GenMarkdownTree(command, os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}
}
