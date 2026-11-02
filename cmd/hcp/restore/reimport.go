package restore

import (
	"fmt"
	"strings"
	"time"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type reimportFlags struct {
	clusterID     string
	replacementMC string
	reason        string
	timeout       time.Duration
}

func NewCmdReimport() *cobra.Command {
	flags := &reimportFlags{}
	cmd := &cobra.Command{
		Use:     "reimport --cluster-id <cluster-id> --replacement-mc <management-cluster> --reason <reason> [--timeout <duration>]",
		Short:   "Move recovered HCP add-on work to a replacement management cluster",
		Long:    "Updates the recovered HCP ManagedCluster and add-on hosting annotations, then deletes old-host add-on ManifestWorks with the Orphan policy. ManagedClusterAddOns are retained.",
		Example: "  osdctl hcp restore reimport --cluster-id ${CLUSTER_ID} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}",
		Args:    cobra.NoArgs, DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) (returnErr error) {
			logger := logrus.New()
			logger.SetOutput(cmd.ErrOrStderr())
			conn, err := utils.CreateConnection()
			if err != nil {
				return fmt.Errorf("creating OCM connection: %w", err)
			}
			defer func() {
				if err := conn.Close(); err != nil && returnErr == nil {
					returnErr = fmt.Errorf("closing OCM connection: %w", err)
				}
			}()
			manager := internalcmd.NewDefaultReimportManager(internalclient.NewOCMClient(conn, internalclient.WithLogger{Logger: logger}))
			plan, err := manager.PrepareReimport(cmd.Context(), internalcmd.ReimportRequest{ClusterID: flags.clusterID, ReplacementMC: flags.replacementMC, Reason: flags.reason, Timeout: flags.timeout})
			if err != nil {
				return err
			}
			addonList := "none"
			if len(plan.Addons) > 0 {
				addonList = strings.Join(plan.Addons, ", ")
			}
			workList := "none"
			if len(plan.OldAddonManifestWorks) > 0 {
				workList = strings.Join(plan.OldAddonManifestWorks, ", ")
			}
			message := fmt.Sprintf("This updates ManagedCluster %s and add-ons (%s) on Service Cluster %s (%s).\nIt deletes old-host add-on ManifestWorks (%s) with the Orphan policy, preserving their applied resources.\nThe ManagedCluster and add-ons will target %s. Continue? [y/N]: ", plan.ManagedClusterName, addonList, plan.ServiceClusterName, plan.ServiceClusterID, workList, plan.ReplacementManagementCluster)
			if !confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), message) {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "Reimport cancelled.")
				return err
			}
			if err := manager.Reimport(cmd.Context(), plan, flags.timeout); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Updated ManagedCluster %s and add-on hosting annotations for management cluster %s; orphaned old-host add-on ManifestWorks.\n", plan.ManagedClusterName, plan.ReplacementManagementCluster)
			return err
		},
	}
	cmd.Flags().StringVarP(&flags.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().StringVar(&flags.replacementMC, "replacement-mc", "", "Replacement management cluster")
	cmd.Flags().StringVar(&flags.reason, "reason", "", "Reason for elevated access")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 5*time.Minute, "Maximum time to wait for deletion")
	for _, flag := range []string{"cluster-id", "replacement-mc", "reason"} {
		if err := cmd.MarkFlagRequired(flag); err != nil {
			panic(fmt.Sprintf("marking flag %q required: %v", flag, err))
		}
	}
	return cmd
}
