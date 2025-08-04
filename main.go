package main

import (
	"fmt"
	"os"

	"github.com/openshift/osdctl/cmd"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"

	metrics "github.com/iamkirkbater/cobra-otel-metrics"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {

	err := osdctlConfig.EnsureConfigFile()
	if err != nil {
		fmt.Println(err)
		return
	}

	stdoutExporter, _ := stdoutmetric.New()

	command := cmd.NewCmdRoot(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})

	command.SetupMetrics(
		metrics.WithExporter(stdoutExporter),
	)

	if err := command.Execute(); err != nil {
		_, err := fmt.Fprintf(os.Stderr, "%v\n", err)
		if err != nil {
			fmt.Println("Error while printing to stderr: ", err.Error())
		}
		os.Exit(1)
	}
}
