package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/openshift/osdctl/cmd"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {

	err := osdctlConfig.EnsureConfigFile()
	if err != nil {
		fmt.Println(err)
		return
	}

	flags := pflag.NewFlagSet("osdctl", pflag.ExitOnError)
	err = flag.CommandLine.Parse([]string{})
	if err != nil {
		fmt.Println("Error parsing commandline flags: ", err.Error())
		return
	}
	pflag.CommandLine = flags

	command := cmd.NewCmdRoot(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})

	if err := command.Execute(); err != nil {
		_, err := fmt.Fprintf(os.Stderr, "%v\n", err)
		if err != nil {
			fmt.Println("Error while printing to stderr: ", err.Error())
		}
		os.Exit(1)
	}
}
