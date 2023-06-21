package cmd

import (
	"fmt"
	"os"
	"strings"

	routev1 "github.com/openshift/api/route/v1"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	gcpv1alpha1 "github.com/openshift/gcp-project-operator/api/v1alpha1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubectl/pkg/util/slice"

	"github.com/openshift/osdctl/cmd/aao"
	"github.com/openshift/osdctl/cmd/account"
	"github.com/openshift/osdctl/cmd/capability"
	"github.com/openshift/osdctl/cmd/cluster"
	"github.com/openshift/osdctl/cmd/clusterdeployment"
	"github.com/openshift/osdctl/cmd/cost"
	"github.com/openshift/osdctl/cmd/env"
	"github.com/openshift/osdctl/cmd/federatedrole"
	"github.com/openshift/osdctl/cmd/jumphost"
	"github.com/openshift/osdctl/cmd/mgmt"
	"github.com/openshift/osdctl/cmd/network"
	"github.com/openshift/osdctl/cmd/org"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/cmd/sts"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
)

func init() {
	_ = awsv1alpha1.AddToScheme(scheme.Scheme)
	_ = routev1.AddToScheme(scheme.Scheme)
	_ = hivev1.AddToScheme(scheme.Scheme)
	_ = gcpv1alpha1.AddToScheme(scheme.Scheme)
}

// NewCmdRoot represents the base command when called without any subcommands
func NewCmdRoot(streams genericclioptions.IOStreams) *cobra.Command {
	globalOpts := &globalflags.GlobalOptions{}
	rootCmd := &cobra.Command{
		Use:               "osdctl",
		Short:             "OSD CLI",
		Long:              `CLI tool to provide OSD related utilities`,
		DisableAutoGenTag: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			skipVersionCheck, err := cmd.Flags().GetBool("skip-version-check")
			if err != nil {
				fmt.Println("flag --skip-version-check/-S undefined")
				os.Exit(1)
			}

			// Checks the skipVersionCheck flag and the command being run to determine if the version check should run
			if shouldRunVersionCheck(skipVersionCheck, cmd.Use) {
				versionCheck()
			}
		},
	}

	globalflags.AddGlobalFlags(rootCmd, globalOpts)
	kubeFlags := globalflags.GetFlags(rootCmd)

	kubeClient := k8s.NewClient(kubeFlags)

	// add sub commands
	rootCmd.AddCommand(aao.NewCmdAao(streams, kubeFlags, kubeClient))
	rootCmd.AddCommand(account.NewCmdAccount(streams, kubeFlags, kubeClient, globalOpts))
	rootCmd.AddCommand(cluster.NewCmdCluster(streams, kubeFlags, kubeClient, globalOpts))
	rootCmd.AddCommand(clusterdeployment.NewCmdClusterDeployment(streams, kubeFlags, kubeClient))
	rootCmd.AddCommand(env.NewCmdEnv(streams, kubeFlags))
	rootCmd.AddCommand(federatedrole.NewCmdFederatedRole(streams, kubeFlags, kubeClient))
	rootCmd.AddCommand(jumphost.NewCmdJumphost())
	rootCmd.AddCommand(mgmt.NewCmdMgmt(streams, kubeFlags, kubeClient, globalOpts))
	rootCmd.AddCommand(network.NewCmdNetwork(streams, kubeFlags, kubeClient))
	rootCmd.AddCommand(servicelog.NewCmdServiceLog())
	rootCmd.AddCommand(org.NewCmdOrg())
	rootCmd.AddCommand(sts.NewCmdSts(streams, kubeFlags, kubeClient))

	// add docs command
	rootCmd.AddCommand(newCmdDocs(streams))

	// add completion command
	rootCmd.AddCommand(newCmdCompletion(streams))

	// add options command to list global flags
	rootCmd.AddCommand(newCmdOptions(streams))

	// Add cost command to use AWS Cost Manager
	rootCmd.AddCommand(cost.NewCmdCost(streams, globalOpts))

	// Add version subcommand. Using the in-build --version flag does not work with cobra
	// because there is no way to hook a function to the --version flag in cobra.
	rootCmd.AddCommand(versionCmd)

	// Add upgradeCmd for upgrading the currently running executable in-place.
	rootCmd.AddCommand(upgradeCmd)

	rootCmd.AddCommand(capability.NewCmdCapability())

	return rootCmd
}

func help(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	if err != nil {
		fmt.Println("Error while printing help: ", err.Error())
	}
}

// Checks if the version check should be run
func shouldRunVersionCheck(skipVersionCheckFlag bool, commandName string) bool {

	// If either are true, then the version check should NOT run, hence negation
	return !(skipVersionCheckFlag || canCommandSkipVersionCheck(commandName))
}

func canCommandSkipVersionCheck(commandName string) bool {
	// Checks if the specific command is in the allowlist
	return slice.ContainsString(getSkipVersionCommands(), commandName, nil)
}

// Returns allowlist of commands that can skip version check
func getSkipVersionCommands() []string {
	return []string{"docs", "upgrade", "version"}
}

func versionCheck() {
	latestVersion, err := utils.GetLatestVersion()
	if err != nil {
		fmt.Println("Warning: Unable to verify that osdctl is running under the latest released version. Error trying to reach GitHub:")
		fmt.Println(err)
		fmt.Println("Please be aware that you are possibly running an outdated or unreleased version.")
	}

	if utils.Version != strings.TrimPrefix(latestVersion, "v") {
		fmt.Printf("The current version (%s) is different than the latest released version (%s).", utils.Version, latestVersion)
		fmt.Println("It is recommended that you update to the latest released version to ensure that no known bugs or issues are hit.")
		fmt.Println("Please confirm that you would like to continue with [y|n]")

		var input string
		for {
			fmt.Scanln(&input)
			if strings.ToLower(input) == "y" {
				break
			}
			if strings.ToLower(input) == "n" {
				fmt.Println("Exiting")
				os.Exit(0)
			}
		}
	}
}
