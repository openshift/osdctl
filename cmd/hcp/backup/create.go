package backup

import (
	_ "embed"
	"fmt"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

//go:embed description.txt
var longDescription string

func NewCmdCreate() *cobra.Command {
	flags := &backupFlags{}

	cmd := &cobra.Command{
		Use:   "create --cluster-id <cluster-id> --reason <reason>",
		Short: "Trigger a Velero backup for an HCP cluster",
		Long:  longDescription,
		Example: "  osdctl hcp backup create --cluster-id ${CLUSTER_ID} --reason ${REASON}\n" +
			"  osdctl hcp backup create --cluster-id ${CLUSTER_ID} --reason ${REASON} --label env=prod --label incident=${REASON}\n" +
			"  osdctl hcp backup create --cluster-id ${CLUSTER_ID} --reason ${REASON} --annotation owner=sre-team",
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

			var creator internalcmd.BackupCreator = internalcmd.NewDefaultBackupCreator(internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: logger}))
			result, err := creator.CreateBackup(cmd.Context(), flags.request())
			if err != nil {
				return err
			}
			if result.ID == "" {
				if _, err := fmt.Fprint(cmd.OutOrStdout(), result.RawOutput); err != nil {
					return err
				}
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "Backup triggered successfully, but could not parse backup ID from velero output.")
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Backup %q triggered successfully.\nTo check status, run:\noc get backup %s -n %s\n", result.ID, result.ID, result.Namespace); err != nil {
				return err
			}
			return nil
		},
	}

	flags.AddFlags(cmd.Flags())
	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}
