package restore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type copySnapshotsFlags struct {
	clusterID       string
	sourceMC        string
	replacementMC   string
	awsProfile      string
	snapshotJournal string
	reason          string
	timeout         time.Duration
	concurrency     int
}

func NewCmdCopySnapshots() *cobra.Command {
	flags := &copySnapshotsFlags{}
	cmd := &cobra.Command{
		Use:     "copy-snapshots --cluster-id <cluster-id> --source-mc <management-cluster> --replacement-mc <management-cluster> --reason <reason>",
		Short:   "Copy HCP EBS snapshots to a replacement management-cluster account",
		Long:    "Discovers EBS VolumeSnapshotContents for the HCP on the replacement management cluster, stores the relevant inventory and snapshot mappings in a restartable journal, copies snapshots through a temporary source-account KMS key, then deletes intermediary snapshots and schedules deletion of the temporary key.",
		Example: "  osdctl hcp restore copy-snapshots --cluster-id ${CLUSTER_ID} --source-mc ${SOURCE_MC} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}",
		Args:    cobra.NoArgs, DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := logrus.New()
			logger.SetOutput(cmd.ErrOrStderr())
			ocmConn, err := utils.CreateConnection()
			if err != nil {
				return fmt.Errorf("creating OCM connection: %w", err)
			}
			defer ocmConn.Close()
			request := internalcmd.CopySnapshotsRequest{ClusterID: flags.clusterID, SourceMC: flags.sourceMC, ReplacementMC: flags.replacementMC, AWSProfile: flags.awsProfile, SnapshotJournalPath: flags.snapshotJournal, Reason: flags.reason, Timeout: flags.timeout, Concurrency: flags.concurrency}
			copier := internalcmd.NewDefaultSnapshotCopierWithLogger(internalclient.NewOCMClient(ocmConn, logger), logger)
			plan, err := copier.PrepareSnapshotCopy(cmd.Context(), request)
			if err != nil {
				return err
			}
			if _, err := os.Stat(plan.SnapshotJournalPath); err == nil {
				if !confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), "A snapshot journal already exists. Resume the previous operation? [y/N] ") {
					journalPath, err := nextSnapshotJournalPath(plan.SnapshotJournalPath)
					if err != nil {
						return err
					}
					plan.SnapshotJournalPath = journalPath
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("checking snapshot journal %s: %w", plan.SnapshotJournalPath, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Discovered %d EBS VolumeSnapshotContents.\n  Region: %s\n  Concurrency: %d\n", len(plan.Entries), plan.Region, plan.Concurrency)
			if !confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), "Create a temporary source KMS key and copy these snapshots? [y/N] ") {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "Snapshot copy cancelled.")
				return err
			}
			copyCtx, cancel := context.WithTimeout(cmd.Context(), flags.timeout)
			defer cancel()
			if err := copier.CopySnapshots(copyCtx, plan); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Copied %d snapshots.\n", len(plan.Entries))
			return err
		},
	}
	cmd.Flags().StringVarP(&flags.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().StringVar(&flags.sourceMC, "source-mc", "", "Internal ID, name, or external ID of the source management cluster")
	cmd.Flags().StringVar(&flags.replacementMC, "replacement-mc", "", "Internal ID, name, or external ID of the replacement management cluster")
	cmd.Flags().StringVarP(&flags.awsProfile, "profile", "p", "", "AWS profile used to assume management-cluster account roles")
	cmd.Flags().StringVar(&flags.snapshotJournal, "snapshot-journal", "", "Path for the restartable snapshot journal (defaults to a cluster-specific file; existing journals prompt to resume or create a numbered file)")
	cmd.Flags().StringVar(&flags.reason, "reason", "", "Reason for elevated access to the replacement management cluster")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 2*time.Hour, "Maximum time to wait for all snapshot copies")
	cmd.Flags().IntVar(&flags.concurrency, "concurrency", 4, "Maximum number of snapshot copy chains to run concurrently")
	for _, flag := range []string{"cluster-id", "source-mc", "replacement-mc", "reason"} {
		_ = cmd.MarkFlagRequired(flag)
	}
	return cmd
}

func nextSnapshotJournalPath(path string) (string, error) {
	extension := filepath.Ext(path)
	base := strings.TrimSuffix(path, extension)
	for suffix := 1; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d%s", base, suffix, extension)
		_, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("checking snapshot journal %s: %w", candidate, err)
		}
	}
}
