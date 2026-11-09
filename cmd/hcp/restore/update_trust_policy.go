package restore

import (
	"fmt"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	hcpflags "github.com/openshift/osdctl/cmd/hcp/internal/flags"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewCmdUpdateTrustPolicy() *cobra.Command {
	flags := &updateTrustPolicyFlags{}
	cmd := &cobra.Command{
		Use: "update-trust-policy --cluster-id <cluster-id> --target-mc <management-cluster>", Short: "Allow a target management cluster to access HCP backups",
		Example: "  osdctl hcp restore update-trust-policy --cluster-id ${CLUSTER_ID} --target-mc ${TARGET_MC}\n  osdctl hcp restore update-trust-policy --cluster-id ${CLUSTER_ID} --source-mc ${SOURCE_MC} --target-mc ${TARGET_MC} --profile ${AWS_PROFILE}",
		Args:    cobra.NoArgs, DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := flags.Validate(); err != nil {
				return err
			}
			logger := logrus.New()
			logger.SetOutput(cmd.ErrOrStderr())
			ocmConn, err := utils.CreateConnection()
			if err != nil {
				return fmt.Errorf("creating OCM connection: %w", err)
			}
			defer ocmConn.Close()
			var updater internalcmd.TrustPolicyUpdater = internalcmd.NewDefaultTrustPolicyUpdater(internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: logger}))
			plan, err := updater.PrepareTrustPolicyUpdate(cmd.Context(), internalcmd.UpdateTrustPolicyRequest{ClusterID: string(flags.Cluster), SourceMC: string(flags.Source), ReplacementMC: string(flags.Destination), AWSProfile: string(flags.AWSProfile)})
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
	flags.RegisterFlags(cmd)
	return cmd
}

type updateTrustPolicyFlags struct {
	hcpflags.Cluster
	hcpflags.Destination
	hcpflags.Source
	hcpflags.AWSProfile
}

func (f *updateTrustPolicyFlags) RegisterFlags(cmd *cobra.Command) {
	f.AddRequired(cmd)
	f.Source.Add(cmd, "Internal ID, name, or external ID of the management cluster currently trusted by the backup role")
	f.Destination.Add(cmd, "Internal ID, name, or external ID of the target management cluster")
	f.AWSProfile.Add(cmd, "AWS profile used to assume the DR account role")
}
