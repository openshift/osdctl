package restore

import (
	"fmt"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type reassignFlags struct{ clusterID, replacementMC string }

func NewCmdReassign() *cobra.Command {
	flags := &reassignFlags{}
	cmd := &cobra.Command{
		Use:               "reassign --cluster-id <cluster-id> --replacement-mc <management-cluster>",
		Short:             "Reassign an HCP to a replacement management cluster in OCM",
		Example:           "  osdctl hcp restore reassign --cluster-id ${CLUSTER_ID} --replacement-mc ${REPLACEMENT_MC}",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := logrus.New()
			logger.SetOutput(cmd.ErrOrStderr())
			ocmConn, err := utils.CreateConnection()
			if err != nil {
				return fmt.Errorf("creating OCM connection: %w", err)
			}
			defer ocmConn.Close()

			manager := internalcmd.NewDefaultReassignmentManager(internalclient.NewOCMClient(ocmConn, logger))
			plan, err := manager.PrepareReassignment(cmd.Context(), internalcmd.ReassignmentRequest{ClusterID: flags.clusterID, ReplacementMC: flags.replacementMC})
			if err != nil {
				return err
			}
			if plan.SourceManagementCluster == plan.ReplacementManagementCluster {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "HCP is already assigned to the replacement management cluster. Nothing to update.")
				return err
			}
			if !confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), fmt.Sprintf("Reassign HCP management cluster in OCM?\nHCP: %s\nSource management cluster: %s (%s)\nReplacement management cluster: %s (%s)\nContinue? [y/N] ", plan.HCPClusterID, plan.SourceManagementCluster, plan.SourceManagementClusterID, plan.ReplacementManagementCluster, plan.ReplacementManagementClusterID)) {
				_, err := fmt.Fprintln(cmd.ErrOrStderr(), "HCP management-cluster reassignment cancelled.")
				return err
			}

			result, err := manager.Reassign(cmd.Context(), plan)
			if err != nil {
				return err
			}
			if result.AlreadyAssigned {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "HCP is already assigned to the replacement management cluster. Nothing to update.")
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "HCP %s reassigned to management cluster %s.\n", plan.HCPClusterID, plan.ReplacementManagementCluster)
			return err
		},
	}
	cmd.Flags().StringVarP(&flags.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().StringVar(&flags.replacementMC, "replacement-mc", "", "Internal ID, name, or external ID of the replacement management cluster")
	for _, flag := range []string{"cluster-id", "replacement-mc"} {
		_ = cmd.MarkFlagRequired(flag)
	}
	return cmd
}
