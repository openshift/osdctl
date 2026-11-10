package restore

import (
	"fmt"
	"strings"
	"time"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	hcpflags "github.com/openshift/osdctl/cmd/hcp/internal/flags"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewCmdReimport() *cobra.Command {
	flags := &reimportFlags{
		Timeout: 5 * time.Minute,
	}
	cmd := &cobra.Command{
		Use:     "reimport --cluster-id <cluster-id> --target-mc <management-cluster> --reason <reason> [--timeout <duration>]",
		Short:   "Move recovered HCP add-on work to a replacement management cluster",
		Long:    "Updates the recovered HCP ManagedCluster and add-on hosting annotations, then deletes old-host add-on ManifestWorks with the Orphan policy. ManagedClusterAddOns are retained.",
		Example: "  osdctl hcp restore reimport --cluster-id ${CLUSTER_ID} --target-mc ${TARGET_MC} --reason ${REASON}",
		Args:    cobra.NoArgs, DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) (returnErr error) {
			if err := flags.Validate(); err != nil {
				return err
			}
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
			plan, err := manager.PrepareReimport(cmd.Context(), internalcmd.ReimportRequest{ClusterID: string(flags.Cluster), ReplacementMC: string(flags.Destination), Reason: string(flags.Reason), Timeout: flags.Timeout})
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
			if err := manager.Reimport(cmd.Context(), plan, flags.Timeout); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Updated ManagedCluster %s and add-on hosting annotations for management cluster %s; orphaned old-host add-on ManifestWorks.\n", plan.ManagedClusterName, plan.ReplacementManagementCluster)
			return err
		},
	}
	flags.RegisterFlags(cmd)
	return cmd
}

type reimportFlags struct {
	hcpflags.Cluster
	hcpflags.Reason
	hcpflags.Destination
	Timeout time.Duration
}

func (f *reimportFlags) RegisterFlags(cmd *cobra.Command) {
	f.Cluster.AddRequired(cmd)
	f.Destination.Add(cmd, "Internal ID, name, or external ID of the target management cluster")
	f.Reason.AddRequired(cmd, "Reason for elevated access")
	cmd.Flags().DurationVar(&f.Timeout, "timeout", f.Timeout, "Maximum time to wait for deletion")
}
