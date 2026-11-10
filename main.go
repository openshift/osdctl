package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		// Stop signal trap when context is done to allow interruption of active prompts
		<-ctx.Done()
		stop()
	}()

	resolved, err := command.ExecuteContextC(ctx)
	if err != nil {
		if resolved != nil && resolved.SilenceErrors {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		if resolved != nil && resolved.SilenceUsage {
			fmt.Fprintf(os.Stderr, "Run '%s --help' for usage.\n", resolved.CommandPath())
		}
		if errors.Is(err, context.Canceled) {
			os.Exit(130) // 128 + SIGINT
		}

		os.Exit(1)
	}
}
