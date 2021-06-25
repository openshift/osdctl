package cmd

import (
	"flag"
	"fmt"
	"os"

	routev1 "github.com/openshift/api/route/v1"
	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	gcpv1alpha1 "github.com/openshift/gcp-project-operator/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/utils/pointer"

	"github.com/openshift/osdctl/cmd/account"
	"github.com/openshift/osdctl/cmd/cluster"
	"github.com/openshift/osdctl/cmd/clusterdeployment"
	"github.com/openshift/osdctl/cmd/cost"
	"github.com/openshift/osdctl/cmd/federatedrole"
	"github.com/openshift/osdctl/cmd/network"
	"github.com/openshift/osdctl/cmd/servicelog"
)

// GitCommit is the short git commit hash from the environment
var GitCommit string

// Version is the tag version from the environment
var Version string

func init() {
	_ = awsv1alpha1.AddToScheme(scheme.Scheme)
	_ = routev1.AddToScheme(scheme.Scheme)
	_ = hivev1.AddToScheme(scheme.Scheme)
	_ = gcpv1alpha1.AddToScheme(scheme.Scheme)
	NewCmdRoot(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
}

// NewCmdRoot represents the base command when called without any subcommands
func NewCmdRoot(streams genericclioptions.IOStreams) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:               "osdctl",
		Version:           fmt.Sprintf("%s, GitCommit: %s", Version, GitCommit),
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
		Impersonate: pointer.StringPtr(""),
	}
	kubeFlags.AddFlags(rootCmd.PersistentFlags())

	// add sub commands
	rootCmd.AddCommand(account.NewCmdAccount(streams, kubeFlags))
	rootCmd.AddCommand(cluster.NewCmdCluster(streams, kubeFlags))
	rootCmd.AddCommand(clusterdeployment.NewCmdClusterDeployment(streams, kubeFlags))
	rootCmd.AddCommand(federatedrole.NewCmdFederatedRole(streams, kubeFlags))
	rootCmd.AddCommand(network.NewCmdNetwork(streams, kubeFlags))
	rootCmd.AddCommand(newCmdMetrics(streams, kubeFlags))
	rootCmd.AddCommand(servicelog.NewCmdServiceLog())

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
