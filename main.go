package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/openshift/osdctl/cmd"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {

	err := osdctlConfig.EnsureConfigFile()
	if err != nil {
		fmt.Println(err)
		return
	}

	latestVersion, err := utils.GetLatestVersion()
	if err != nil {
		fmt.Println("Warning: Unable to verify that osdctl is running under the latest version. Error trying to reach GitHub:")
		fmt.Println(err)
		fmt.Println("Please be aware that you are possibly running an outdated version.")

		// Version query failed, so we just assume that the version didn't change
		latestVersion = utils.Version
	}

	if utils.Version != latestVersion && utils.Version != latestVersion+"-next" {
		fmt.Println("The current version is different than the latest version.")
		fmt.Println("It is recommended that you update to the latest version to ensure that no known bugs or issues are hit.")
		fmt.Println("Please confirm that you would like to continue with [y|n]")

		var input string
		for {
			fmt.Scanln(&input)
			if strings.ToLower(input) == "y" {
				break
			}
			if strings.ToLower(input) == "n" {
				fmt.Println("Exiting")
				return
			}
			fmt.Println("Input not recognized. Please select [y|n]")
		}
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
