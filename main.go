package main

import (
	"fmt"
	"os"

	"github.com/emicklei/go-restful/v3/log"
	"github.com/openshift/osdctl/cmd"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {

	err := osdctlConfig.EnsureConfigFile()
	if err != nil {
		fmt.Println(err)
		return
	}

	command := cmd.NewCmdRoot(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})

	// Initialize the logger - this silences the warning and enables logging
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	if err := command.Execute(); err != nil {
		_, err := fmt.Fprintf(os.Stderr, "%v\n", err)
		if err != nil {
			fmt.Println("Error while printing to stderr: ", err.Error())
		}
		os.Exit(1)
	}

}
