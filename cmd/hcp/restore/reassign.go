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

func NewCmdReassign() *cobra.Command {
	flags := &reassignFlags{}
	cmd := &cobra.Command{
		Use:               "reassign --cluster-id <cluster-id> --target-mc <management-cluster>",
		Short:             "Reassign an HCP to a replacement management cluster in OCM",
		Example:           "  osdctl hcp restore reassign --cluster-id ${CLUSTER_ID} --target-mc ${TARGET_MC}",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
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

			manager := internalcmd.NewDefaultReassignmentManager(internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: logger}))
			plan, err := manager.PrepareReassignment(cmd.Context(), internalcmd.ReassignmentRequest{ClusterID: string(flags.Cluster), ReplacementMC: string(flags.Destination)})
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
	flags.RegisterFlags(cmd)
	return cmd
}

type reassignFlags struct {
	hcpflags.Cluster
	hcpflags.Destination
}

func (f *reassignFlags) RegisterFlags(cmd *cobra.Command) {
	f.AddRequired(cmd)
	f.Destination.Add(cmd, "Internal ID, name, or external ID of the target management cluster")
}
