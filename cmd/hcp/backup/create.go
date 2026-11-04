package backup

import (
	_ "embed"
	"fmt"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	hcpflags "github.com/openshift/osdctl/cmd/hcp/internal/flags"
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

	flags.RegisterFlags(cmd)

	return cmd
}

// backupFlags holds the parsed command-line flag values for the backup command.
type backupFlags struct {
	hcpflags.Cluster
	hcpflags.Reason
	// labels holds optional key=value pairs that are forwarded to the Velero
	// backup CR via --labels. Populated by repeated --label flags.
	labels map[string]string
	// annotations holds optional key=value pairs that are forwarded to the
	// Velero backup CR via --annotations. Populated by repeated --annotation flags.
	annotations map[string]string
}

// RegisterFlags binds the command-line flags for the backup command.
func (f *backupFlags) RegisterFlags(cmd *cobra.Command) {
	f.Cluster.AddRequired(cmd)
	f.Reason.AddRequired(cmd, "Reason for privilege elevation (e.g., OHSS-1234 or PD incident ID)")
	flags := cmd.Flags()
	flags.StringToStringVar(&f.labels, "label", nil, "Label to add to the Velero Backup CR (key=value); may be repeated")
	flags.StringToStringVar(&f.annotations, "annotation", nil, "Annotation to add to the Velero Backup CR (key=value); may be repeated")
}

func (f *backupFlags) request() internalcmd.BackupRequest {
	return internalcmd.BackupRequest{
		ClusterID:   string(f.Cluster),
		Reason:      string(f.Reason),
		Labels:      f.labels,
		Annotations: f.annotations,
	}
}
