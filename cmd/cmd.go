package cmd

import (
	"fmt"
	"os"
	"strings"

	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	gcpv1alpha1 "github.com/openshift/gcp-project-operator/api/v1alpha1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubectl/pkg/util/slice"

	"github.com/openshift/osdctl/cmd/aao"
	"github.com/openshift/osdctl/cmd/account"
	"github.com/openshift/osdctl/cmd/alerts"
	"github.com/openshift/osdctl/cmd/capability"
	"github.com/openshift/osdctl/cmd/cloudtrail"
	"github.com/openshift/osdctl/cmd/cluster"
	"github.com/openshift/osdctl/cmd/cost"
	"github.com/openshift/osdctl/cmd/env"
	"github.com/openshift/osdctl/cmd/hive"
	"github.com/openshift/osdctl/cmd/iampermissions"
	"github.com/openshift/osdctl/cmd/jira"
	"github.com/openshift/osdctl/cmd/jumphost"
	"github.com/openshift/osdctl/cmd/mc"
	"github.com/openshift/osdctl/cmd/network"
	"github.com/openshift/osdctl/cmd/org"
	"github.com/openshift/osdctl/cmd/promote"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
)

func init() {
	_ = operatorv1.AddToScheme(scheme.Scheme)
	_ = awsv1alpha1.AddToScheme(scheme.Scheme)
	_ = routev1.AddToScheme(scheme.Scheme)
	_ = hivev1.AddToScheme(scheme.Scheme)
	_ = gcpv1alpha1.AddToScheme(scheme.Scheme)
	_ = hivev1alpha1.AddToScheme(scheme.Scheme)
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
			noAwsProxy, err := cmd.Flags().GetBool(aws.NoProxyFlag)
			if err != nil {
				fmt.Printf("flag --%v undefined\n", aws.NoProxyFlag)
				os.Exit(1)
			}
			viper.Set(aws.NoProxyFlag, noAwsProxy)

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
	rootCmd.AddCommand(aao.NewCmdAao(kubeClient))
	rootCmd.AddCommand(account.NewCmdAccount(streams, kubeClient, globalOpts))
	rootCmd.AddCommand(alerts.NewCmdAlerts())
	rootCmd.AddCommand(cluster.NewCmdCluster(streams, kubeClient, globalOpts))
	rootCmd.AddCommand(hive.NewCmdHive(streams, kubeClient))
	rootCmd.AddCommand(env.NewCmdEnv())
	rootCmd.AddCommand(jumphost.NewCmdJumphost())
	rootCmd.AddCommand(mc.NewCmdMC())
	rootCmd.AddCommand(network.NewCmdNetwork(streams, kubeClient))
	rootCmd.AddCommand(servicelog.NewCmdServiceLog())
	rootCmd.AddCommand(org.NewCmdOrg())
	rootCmd.AddCommand(promote.NewCmdPromote())
	rootCmd.AddCommand(jira.Cmd)
	rootCmd.AddCommand(cloudtrail.NewCloudtrailCmd())
	rootCmd.AddCommand(iampermissions.NewCmdIamPermissions())
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
	return []string{"upgrade", "version"}
}

func versionCheck() {
	latestVersion, err := utils.GetLatestVersion()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "WARN: Unable to verify that osdctl is running under the latest released version. Error trying to reach GitHub:")
		_, _ = fmt.Fprintln(os.Stderr, err)
		_, _ = fmt.Fprintln(os.Stderr, "Please be aware that you are possibly running an outdated or unreleased version.")
	}

	if utils.Version != strings.TrimPrefix(latestVersion, "v") {
		_, _ = fmt.Fprintf(os.Stderr, "WARN: The current version (%s) is different than the latest released version (%s). It is recommended that you update to the latest released version to ensure that no known bugs or issues are hit.\n", utils.Version, latestVersion)

		if !utils.ConfirmPrompt() {
			os.Exit(0)
		}
	}
}
