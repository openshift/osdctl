package cmd

import (
	"flag"
	"os"

	routev1 "github.com/openshift/api/route/v1"
	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/openshift/osd-utils-cli/cmd/list"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/utils/pointer"
)

// GitCommit is the short git commit hash from the environment
var GitCommit string

func init() {
	awsv1alpha1.AddToScheme(scheme.Scheme)
	routev1.AddToScheme(scheme.Scheme)

	NewCmdRoot(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
}

// rootCmd represents the base command when called without any subcommands
func NewCmdRoot(streams genericclioptions.IOStreams) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:               "osdctl",
		Version:           GitCommit,
		Short:             "OSD CLI",
		Long:              `CLI tool to provide OSD related utilities`,
		DisableAutoGenTag: true,
		Run:               help,
	}

	rootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	// Reuse kubectl global flags to provide namespace, context and credential options.
	// We are not using NewConfigFlags here to avoid adding too many flags
	kubeFlags := &genericclioptions.ConfigFlags{
		KubeConfig:  pointer.StringPtr(""),
		ClusterName: pointer.StringPtr(""),
		Context:     pointer.StringPtr(""),
		Namespace:   pointer.StringPtr(""),
		APIServer:   pointer.StringPtr(""),
		Timeout:     pointer.StringPtr("0"),
		Insecure:    pointer.BoolPtr(false),
	}
	kubeFlags.AddFlags(rootCmd.PersistentFlags())

	// add sub commands
	rootCmd.AddCommand(newCmdReset(streams, kubeFlags))
	rootCmd.AddCommand(newCmdSet(streams, kubeFlags))
	rootCmd.AddCommand(list.NewCmdList(streams, kubeFlags))
	rootCmd.AddCommand(newCmdConsole(streams, kubeFlags))
	rootCmd.AddCommand(newCmdMetrics(streams, kubeFlags))
	rootCmd.AddCommand(newCmdCleanVeleroSnapshots(streams))

	// add docs command
	rootCmd.AddCommand(newCmdDocs(streams))

	// add options command to list global flags
	templates.ActsAsRootCommand(rootCmd, []string{"options"})
	rootCmd.AddCommand(newCmdOptions(streams))

	return rootCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
