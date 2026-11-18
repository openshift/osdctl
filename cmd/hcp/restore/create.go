package restore

import (
	"fmt"
	"strings"
	"time"

	"charm.land/huh/v2"
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	hcpflags "github.com/openshift/osdctl/cmd/hcp/internal/flags"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewCmdCreate() *cobra.Command {
	flags := &createFlags{
		Timeout: 30 * time.Minute,
	}
	cmd := &cobra.Command{
		Use:   "create --cluster-id <cluster-id> --reason <reason> [--backup-name <backup-name>] [--target-mc <management-cluster>] [--timeout <duration>]",
		Short: "Create and monitor a Velero restore for an HCP backup",
		Long:  "Create and monitor a Velero Restore in the target management cluster. The target defaults to the management cluster currently bound to the HCP; an explicit target must be in the HCP's region. Repeated invocations safely resume an active restore or report an already completed restore; failed attempts receive a new retry Restore. If backup-name is not provided, an interactive prompt will display available backups.",
		Example: "  osdctl hcp restore create --cluster-id ${CLUSTER_ID} --reason ${REASON}\n" +
			"  osdctl hcp restore create --cluster-id ${CLUSTER_ID} --backup-name ${BACKUP_NAME} --reason ${REASON}\n" +
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

			if strings.TrimSpace(flags.BackupName) == "" {
				var discoverer internalcmd.BackupDiscoverer = internalcmd.NewDefaultBackupDiscoverer(internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: logger}))
				backups, err := discoverer.DiscoverBackups(cmd.Context(), internalcmd.DiscoveryRequest{
					ClusterID: string(flags.Cluster), Limit: 5, AWSProfile: "",
				})
				if err != nil {
					return err
				}
				if len(backups) > 0 {
					// Convert backups to huh options with formatted timestamps for readability
					options := make([]huh.Option[string], 0, len(backups))
					for _, backup := range backups {
						// Format: "2026-11-17 22:00:14 - backup-id"
						label := fmt.Sprintf("%s - %s", backup.Timestamp.Format("2006-01-02 15:04:05"), backup.ID)
						options = append(options, huh.NewOption(label, backup.ID))
					}

					var selectedBackup string
					prompt := huh.NewForm(
						huh.NewGroup(
							huh.NewSelect[string]().
								Title("Select Backup").
								Options(options...).
								Value(&selectedBackup),
						),
					)

					err := prompt.Run()
					if err != nil {
						return fmt.Errorf("unable to select backup name: %w", err)
					}
					flags.BackupName = selectedBackup
				} else {
					return fmt.Errorf("no backups found for cluster")
				}
			}

			creator := internalcmd.NewDefaultRestoreCreator(internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: logger}))
			result, err := creator.CreateRestore(cmd.Context(), internalcmd.RestoreRequest{ClusterID: string(flags.Cluster), TargetMC: string(flags.Destination), BackupName: flags.BackupName, Reason: string(flags.Reason), Timeout: flags.Timeout})
			if err != nil {
				return err
			}
			if result.Created {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created Velero Restore %s for backup %s.\n", result.Name, flags.BackupName)
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Monitoring existing Velero Restore %s for backup %s.\n", result.Name, flags.BackupName)
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Velero Restore %s completed.\n", result.Name)
			return err
		},
	}
	flags.RegisterFlags(cmd)
	return cmd
}

type createFlags struct {
	hcpflags.Cluster
	hcpflags.Reason
	hcpflags.Destination
	BackupName string
	Timeout    time.Duration
}

func (f *createFlags) RegisterFlags(cmd *cobra.Command) {
	f.Cluster.AddRequired(cmd)
	f.Reason.AddRequired(cmd, "Reason for elevated access")
	flags := cmd.Flags()
	f.Destination.Add(cmd, "Internal ID, name, or external ID of the management cluster that will run the restore")
	flags.StringVar(&f.BackupName, "backup-name", f.BackupName, "Name of the completed Velero backup to restore (optional - will prompt if not provided)")
	flags.DurationVar(&f.Timeout, "timeout", f.Timeout, "Maximum time to wait for the restore to complete")
}
