package cmd

import (
	"flag"
	"os"

	routev1 "github.com/openshift/api/route/v1"
	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/utils/pointer"

	"github.com/openshift/osd-utils-cli/cmd/account"
	"github.com/openshift/osd-utils-cli/cmd/clusterdeployment"
	"github.com/openshift/osd-utils-cli/cmd/cost"
	"github.com/openshift/osd-utils-cli/cmd/federatedrole"
)

// GitCommit is the short git commit hash from the environment
var GitCommit string

func init() {
	_ = awsv1alpha1.AddToScheme(scheme.Scheme)
	_ = routev1.AddToScheme(scheme.Scheme)
	_ = hivev1.AddToScheme(scheme.Scheme)

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
		APIServer:   pointer.StringPtr(""),
		Timeout:     pointer.StringPtr("0"),
		Insecure:    pointer.BoolPtr(false),
	}
	kubeFlags.AddFlags(rootCmd.PersistentFlags())

	// add sub commands
	rootCmd.AddCommand(account.NewCmdAccount(streams, kubeFlags))
	rootCmd.AddCommand(clusterdeployment.NewCmdClusterDeployment(streams, kubeFlags))
	rootCmd.AddCommand(federatedrole.NewCmdFederatedRole(streams, kubeFlags))
	rootCmd.AddCommand(newCmdMetrics(streams, kubeFlags))

	// add docs command
	rootCmd.AddCommand(newCmdDocs(streams))

	// add completion command
	rootCmd.AddCommand(newCmdCompletion(streams))

	// add options command to list global flags
	templates.ActsAsRootCommand(rootCmd, []string{"options"})
	rootCmd.AddCommand(newCmdOptions(streams))

	//Add cost command to use AWS Cost Manager
	rootCmd.AddCommand(cost.NewCmdCost(streams))

	return rootCmd
}

func help(cmd *cobra.Command, _ []string) {
	cmd.Help()
}
