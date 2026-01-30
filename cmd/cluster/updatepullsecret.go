package cluster

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/openshift/osdctl/internal/utils/globalflags"
)

const updatePullSecCmdExample = `
  # Update Pull Secret's OCM access token data
  osdctl cluster update-pull-secret --cluster-id 1kfmyclusteristhebesteverp8m --reason "Update PullSecret per pd or jira-id"
`

func newCmdUpdatePullSecret(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newTransferOwnerOptions(streams, globalOpts)
	updatePullSecretCmd := &cobra.Command{
		Use:               "update-pull-secret",
		Short:             "Update cluster pullsecret with current OCM accessToken data(to be done by Region Lead)",
		Long:              fmt.Sprintf("Update cluster pullsecret with current OCM accessToken data(to be done by Region Lead)\n\n%s\n", transferOwnerDocs),
		Args:              cobra.NoArgs,
		Example:           updatePullSecCmdExample,
		DisableAutoGenTag: true,
		PreRun:            func(cmd *cobra.Command, args []string) { cmdutil.CheckErr(ops.preRun()) },
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.run())
		},
	}
	// can we get cluster-id from some context maybe?
	updatePullSecretCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "The Internal Cluster ID/External Cluster ID/ Cluster Name")
	updatePullSecretCmd.Flags().BoolVarP(&ops.dryrun, "dry-run", "d", false, "Dry-run - show all changes but do not apply them")
	updatePullSecretCmd.Flags().StringVar(&ops.reason, "reason", "", "The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)")

	_ = updatePullSecretCmd.MarkFlagRequired("cluster-id")
	_ = updatePullSecretCmd.MarkFlagRequired("reason")
	// This arg is used as part of this wrapper command to instruct the transfer op to exit after
	// updating the pull secret, and before doing the ownership transfer...
	updatePullSecretCmd.Flags().BoolVar(&ops.doPullSecretOnly, "pull-secret-only", true, "Update cluster pull secret from current OCM AccessToken data without ownership transfer")
	_ = updatePullSecretCmd.Flags().MarkHidden("pull-secret-only")

	return updatePullSecretCmd
}
