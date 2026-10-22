package restore

import (
	"fmt"
	"time"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type createFlags struct {
	clusterID  string
	targetMC   string
	backupName string
	reason     string
	timeout    time.Duration
}

func NewCmdCreate() *cobra.Command {
	flags := &createFlags{}
	cmd := &cobra.Command{
		Use:   "create --cluster-id <cluster-id> --backup-name <backup-name> --reason <reason> [--target-mc <management-cluster>] [--timeout <duration>]",
		Short: "Create and monitor a Velero restore for an HCP backup",
		Long:  "Create and monitor a Velero Restore in the target management cluster. The target defaults to the management cluster currently bound to the HCP; an explicit target must be in the HCP's region. Repeated invocations safely resume an active restore or report an already completed restore; failed attempts receive a new retry Restore.",
		Example: "  osdctl hcp restore create --cluster-id ${CLUSTER_ID} --backup-name ${BACKUP_NAME} --reason ${REASON}\n" +
			"  osdctl hcp restore create --cluster-id ${CLUSTER_ID} --target-mc ${TARGET_MC} --backup-name ${BACKUP_NAME} --reason ${REASON} --timeout 30m",
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

			creator := internalcmd.NewDefaultRestoreCreator(internalclient.NewOCMClient(ocmConn, logger))
			result, err := creator.CreateRestore(cmd.Context(), internalcmd.RestoreRequest{ClusterID: flags.clusterID, TargetMC: flags.targetMC, BackupName: flags.backupName, Reason: flags.reason, Timeout: flags.timeout})
			if err != nil {
				return err
			}
			if result.Created {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created Velero Restore %s for backup %s.\n", result.Name, flags.backupName)
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Monitoring existing Velero Restore %s for backup %s.\n", result.Name, flags.backupName)
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Velero Restore %s completed.\n", result.Name)
			return err
		},
	}
	cmd.Flags().StringVarP(&flags.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().StringVar(&flags.targetMC, "target-mc", "", "Internal ID, name, or external ID of the management cluster that will run the restore")
	cmd.Flags().StringVar(&flags.backupName, "backup-name", "", "Name of the completed Velero backup to restore")
	cmd.Flags().StringVar(&flags.reason, "reason", "", "Reason for elevated access")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 30*time.Minute, "Maximum time to wait for the restore to complete")
	for _, flag := range []string{"cluster-id", "backup-name", "reason"} {
		_ = cmd.MarkFlagRequired(flag)
	}
	return cmd
}
