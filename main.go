package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/openshift/osdctl/cmd"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	srelibpkg "github.com/openshift/osdctl/pkg/srelib"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func resolveSrelibPlugin() string {
	if p := os.Getenv("SRELIB_PLUGIN_PATH"); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "srelib-plugin")
}

func main() {

	err := osdctlConfig.EnsureConfigFile()
	if err != nil {
		fmt.Println(err)
		return
	}

	pluginPath := resolveSrelibPlugin()
	srelibClient, err := srelibpkg.NewClient(pluginPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: cannot start srelib plugin (%s): %v\n", pluginPath, err)
		os.Exit(1)
	}
	defer srelibClient.Close()
	utils.SetSrelibClient(srelibClient)

	cobra.EnableTraverseRunHooks = true
	command := cmd.NewCmdRoot(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})

	if err := command.Execute(); err != nil {
		_, err := fmt.Fprintf(os.Stderr, "%v\n", err)
		if err != nil {
			fmt.Println("Error while printing to stderr: ", err.Error())
		}
		os.Exit(1)
	}
}
