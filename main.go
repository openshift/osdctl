package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/openshift/osd-utils-cli/cmd"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/component-base/logs"
)

func main() {
	flags := pflag.NewFlagSet("osd-utils-cli", pflag.ExitOnError)
	flag.CommandLine.Parse([]string{})
	pflag.CommandLine = flags

	logs.InitLogs()
	defer logs.FlushLogs()

	command := cmd.NewCmdRoot(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})

	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
