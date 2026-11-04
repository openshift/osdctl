package restore

import (
	"fmt"
	"time"

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
	flags.StringVar(&f.BackupName, "backup-name", f.BackupName, "Name of the completed Velero backup to restore")
	flags.DurationVar(&f.Timeout, "timeout", f.Timeout, "Maximum time to wait for the restore to complete")
	if err := cmd.MarkFlagRequired("backup-name"); err != nil {
		panic(fmt.Sprintf("marking flag %q required: %v", "backup-name", err))
	}
}
