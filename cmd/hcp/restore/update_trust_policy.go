package restore

import (
	"fmt"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type updateTrustPolicyFlags struct{ clusterID, sourceMC, replacementMC, awsProfile string }

func NewCmdUpdateTrustPolicy() *cobra.Command {
	flags := &updateTrustPolicyFlags{}
	cmd := &cobra.Command{
		Use: "update-trust-policy --cluster-id <cluster-id> --replacement-mc <management-cluster>", Short: "Allow a replacement management cluster to access HCP backups",
		Example: "  osdctl hcp restore update-trust-policy --cluster-id ${CLUSTER_ID} --replacement-mc ${REPLACEMENT_MC}\n  osdctl hcp restore update-trust-policy --cluster-id ${CLUSTER_ID} --source-mc ${SOURCE_MC} --replacement-mc ${REPLACEMENT_MC} --profile ${AWS_PROFILE}",
		Args:    cobra.NoArgs, DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := logrus.New()
			logger.SetOutput(cmd.ErrOrStderr())
			ocmConn, err := utils.CreateConnection()
			if err != nil {
				return fmt.Errorf("creating OCM connection: %w", err)
			}
			defer ocmConn.Close()
			var updater internalcmd.TrustPolicyUpdater = internalcmd.NewDefaultTrustPolicyUpdater(internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: logger}))
			plan, err := updater.PrepareTrustPolicyUpdate(cmd.Context(), internalcmd.UpdateTrustPolicyRequest{ClusterID: flags.clusterID, SourceMC: flags.sourceMC, ReplacementMC: flags.replacementMC, AWSProfile: flags.awsProfile})
			if err != nil {
				return err
			}
			if plan.RoleName == "" {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "Backup role trust policy already trusts the replacement management cluster. Nothing to update.")
				return err
			}
			if !confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), fmt.Sprintf("Update backup role trust policy?\nRole: %s\nSource management cluster: %s (%s)\nReplacement management cluster: %s (%s)\nContinue? [y/N] ", plan.RoleName, plan.SourceManagementCluster, plan.SourceManagementClusterID, plan.ReplacementManagementCluster, plan.ReplacementManagementClusterID)) {
				_, err := fmt.Fprintln(cmd.ErrOrStderr(), "Backup role trust policy update cancelled.")
				return err
			}
			return updater.UpdateTrustPolicy(cmd.Context(), plan)
		},
	}
	cmd.Flags().StringVarP(&flags.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().StringVar(&flags.sourceMC, "source-mc", "", "Internal ID, name, or external ID of the management cluster currently trusted by the backup role")
	cmd.Flags().StringVar(&flags.replacementMC, "replacement-mc", "", "Internal ID, name, or external ID of the replacement management cluster")
	cmd.Flags().StringVarP(&flags.awsProfile, "profile", "p", "", "AWS profile used to assume the DR account role")
	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("replacement-mc")
	return cmd
}
