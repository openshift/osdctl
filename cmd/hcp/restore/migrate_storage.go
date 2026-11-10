package restore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	hcpflags "github.com/openshift/osdctl/cmd/hcp/internal/flags"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewCmdMigrateStorage() *cobra.Command {
	flags := &migrateStorageFlags{
		Timeout:     2 * time.Hour,
		Concurrency: 4,
	}
	cmd := &cobra.Command{
		Use:     "migrate-storage --cluster-id <cluster-id> --target-mc <management-cluster> --reason <reason>",
		Short:   "Migrate HCP snapshot storage to a replacement management cluster",
		Long:    "Discovers EBS VolumeSnapshotContents for the HCP on the replacement management cluster, stores the relevant inventory and snapshot mappings in a restartable journal, copies snapshots through a temporary source-account KMS key, replaces the target VSCs with the copied snapshot IDs, and resumes the control plane only after all replacement VSCs are ready to use.",
		Example: "  osdctl hcp restore migrate-storage --cluster-id ${CLUSTER_ID} --target-mc ${TARGET_MC} --reason ${REASON}",
		Args:    cobra.NoArgs, DisableAutoGenTag: true,
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
			request := internalcmd.CopySnapshotsRequest{ClusterID: string(flags.Cluster), SourceMC: string(flags.Source), ReplacementMC: string(flags.Destination), AWSProfile: string(flags.AWSProfile), SnapshotJournalPath: flags.SnapshotJournal, Reason: string(flags.Reason), Timeout: flags.Timeout, Concurrency: flags.Concurrency}
			copier := internalcmd.NewDefaultSnapshotCopier(internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: logger}), internalclient.NewAWSAccountClientProvider(), internalcmd.WithLogger{Logger: logger})
			plan, err := copier.PrepareSnapshotCopy(cmd.Context(), request)
			if err != nil {
				return err
			}
			journalPath, err := selectSnapshotJournal(cmd.InOrStdin(), cmd.OutOrStdout(), plan.ClusterID, requestedSnapshotJournalPath(flags.SnapshotJournal, plan.SnapshotJournalPath), plan.SnapshotJournalPath)
			if err != nil {
				return err
			}
			plan.SnapshotJournalPath = journalPath
			if err := internalcmd.ReconcileSnapshotCopyPlan(cmd.Context(), &plan); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Discovered %d EBS VolumeSnapshotContents.\n  Source management cluster: %s\n  Replacement management cluster: %s\n  Region: %s\n  Concurrency: %d\n", len(plan.Entries), plan.SourceManagementCluster, plan.ReplacementManagementCluster, plan.Region, plan.Concurrency)
			if !confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), "Create a temporary source KMS key and copy these snapshots? [y/N] ") {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "Snapshot copy cancelled.")
				return err
			}
			copyCtx, cancel := context.WithTimeout(cmd.Context(), flags.Timeout)
			defer cancel()
			if err := copier.CopySnapshots(copyCtx, plan); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Copied %d snapshots.\n", len(plan.Entries))
			return err
		},
	}
	flags.RegisterFlags(cmd)
	return cmd
}

type migrateStorageFlags struct {
	hcpflags.Cluster
	hcpflags.Reason
	hcpflags.Destination
	hcpflags.Source
	hcpflags.AWSProfile
	SnapshotJournal string
	Timeout         time.Duration
	Concurrency     int
}

func (f *migrateStorageFlags) RegisterFlags(cmd *cobra.Command) {
	f.Cluster.AddRequired(cmd)
	f.Source.Add(cmd, "Internal ID, name, or external ID of the source management cluster (defaults to the HCP's current management cluster)")
	f.Destination.Add(cmd, "Internal ID, name, or external ID of the target management cluster")
	f.AWSProfile.Add(cmd, "AWS profile used to assume management-cluster account roles")
	flags := cmd.Flags()
	flags.StringVar(&f.SnapshotJournal, "snapshot-journal", f.SnapshotJournal, "Path for the restartable snapshot journal (defaults to a cluster-specific file; existing journals prompt to resume or create a numbered file)")
	f.Reason.AddRequired(cmd, "Reason for elevated access to the replacement management cluster")
	flags.DurationVar(&f.Timeout, "timeout", f.Timeout, "Maximum time to wait for all snapshot copies")
	flags.IntVar(&f.Concurrency, "concurrency", f.Concurrency, "Maximum number of snapshot copy chains to run concurrently")
}

func requestedSnapshotJournalPath(requestedPath, resolvedPath string) string {
	if requestedPath == "" {
		return ""
	}
	return resolvedPath
}

type snapshotJournalSummary struct {
	Path     string
	Location string
	Source   string
	Target   string
	Complete int
	Total    int
}

func selectSnapshotJournal(in io.Reader, out io.Writer, clusterID, requestedPath, defaultPath string) (string, error) {
	if requestedPath != "" {
		return selectExistingJournal(in, out, requestedPath, defaultPath)
	}

	paths, err := discoverSnapshotJournals(clusterID)
	if err != nil {
		return "", err
	}
	return selectSnapshotJournalFromPaths(in, out, paths, defaultPath)
}

func selectSnapshotJournalFromPaths(in io.Reader, out io.Writer, paths []string, defaultPath string) (string, error) {
	if len(paths) == 0 {
		return defaultPath, nil
	}
	if len(paths) == 1 {
		return selectExistingJournal(in, out, paths[0], defaultPath)
	}

	if _, err := fmt.Fprintln(out, "Snapshot journals found:"); err != nil {
		return "", fmt.Errorf("writing snapshot journal list: %w", err)
	}
	for index, path := range paths {
		summary := readSnapshotJournalSummary(path)
		if _, err := fmt.Fprintf(out, "  %d. %s [%s] (%s → %s, %d/%d copied)\n", index+1, summary.Path, summary.Location, summary.Source, summary.Target, summary.Complete, summary.Total); err != nil {
			return "", fmt.Errorf("writing snapshot journal list: %w", err)
		}
	}
	if _, err := fmt.Fprint(out, "Select a journal to resume, or enter 0 to start a new operation: "); err != nil {
		return "", fmt.Errorf("writing snapshot journal prompt: %w", err)
	}
	var selection int
	if _, err := fmt.Fscanln(in, &selection); err != nil {
		return "", fmt.Errorf("selecting snapshot journal: %w", err)
	}
	if selection == 0 {
		return nextSnapshotJournalPath(defaultPath)
	}
	if selection < 1 || selection > len(paths) {
		return "", fmt.Errorf("snapshot journal selection must be between 0 and %d", len(paths))
	}
	return paths[selection-1], nil
}

func selectExistingJournal(in io.Reader, out io.Writer, path, defaultPath string) (string, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("checking snapshot journal %s: %w", path, err)
	}
	if confirm(in, out, fmt.Sprintf("Resume snapshot journal %s? [y/N] ", journalDisplayName(path))) {
		return path, nil
	}
	return nextSnapshotJournalPath(defaultPath)
}

func discoverSnapshotJournals(clusterID string) ([]string, error) {
	directories := []string{"."}
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		directories = append(directories, filepath.Join(dataHome, "osdctl"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		directories = append(directories, filepath.Join(home, ".local", "share", "osdctl"))
	}
	return discoverSnapshotJournalsIn(directories, clusterID)
}

func discoverSnapshotJournalsIn(directories []string, clusterID string) ([]string, error) {
	name := fmt.Sprintf("snapshot-copy-journal-%s.json", clusterID)
	base := strings.TrimSuffix(name, filepath.Ext(name))
	seen := make(map[string]struct{})
	var paths []string
	for _, directory := range directories {
		entries, err := os.ReadDir(directory)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading snapshot journal directory: %w", err)
		}
		addPath := func(entryName string) error {
			path := filepath.Join(directory, entryName)
			absolute, err := filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("resolving snapshot journal path: %w", err)
			}
			if _, ok := seen[absolute]; !ok {
				seen[absolute] = struct{}{}
				paths = append(paths, path)
			}
			return nil
		}
		for _, entry := range entries {
			if entry.Name() == name {
				if err := addPath(entry.Name()); err != nil {
					return nil, err
				}
			}
		}
		for _, entry := range entries {
			if !isNumberedSnapshotJournal(entry.Name(), base) {
				continue
			}
			if err := addPath(entry.Name()); err != nil {
				return nil, err
			}
		}
	}
	return paths, nil
}

func isNumberedSnapshotJournal(name, base string) bool {
	suffix := strings.TrimSuffix(strings.TrimPrefix(name, base+"-"), ".json")
	if suffix == "" || name != base+"-"+suffix+".json" {
		return false
	}
	for _, character := range suffix {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func readSnapshotJournalSummary(path string) snapshotJournalSummary {
	summary := snapshotJournalSummary{Path: journalDisplayName(path), Location: journalLocation(path), Source: "unknown source", Target: "unknown target"}
	data, err := os.ReadFile(path)
	if err != nil {
		return summary
	}
	var journal struct {
		SourceAccountID      string                          `json:"sourceAccountID"`
		ReplacementAccountID string                          `json:"replacementAccountID"`
		Entries              []internalcmd.SnapshotCopyEntry `json:"entries"`
	}
	if json.Unmarshal(data, &journal) != nil {
		return summary
	}
	if journal.SourceAccountID != "" {
		summary.Source = redactedAccountID(journal.SourceAccountID)
	}
	if journal.ReplacementAccountID != "" {
		summary.Target = redactedAccountID(journal.ReplacementAccountID)
	}
	summary.Total = len(journal.Entries)
	for _, entry := range journal.Entries {
		if entry.DestinationSnapshotID != "" {
			summary.Complete++
		}
	}
	return summary
}

func journalDisplayName(path string) string {
	return filepath.Base(path)
}

func journalLocation(path string) string {
	directory := filepath.Dir(path)
	if sameDirectory(directory, ".") {
		return "current directory"
	}
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" && sameDirectory(directory, filepath.Join(dataHome, "osdctl")) {
		return "XDG data directory"
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && sameDirectory(directory, filepath.Join(home, ".local", "share", "osdctl")) {
		return "home data directory"
	}
	return fmt.Sprintf("directory %q", filepath.Base(directory))
}

func sameDirectory(first, second string) bool {
	firstPath, firstErr := filepath.Abs(first)
	secondPath, secondErr := filepath.Abs(second)
	return firstErr == nil && secondErr == nil && firstPath == secondPath
}

func redactedAccountID(accountID string) string {
	if len(accountID) <= 4 {
		return "redacted account"
	}
	return fmt.Sprintf("account ending %s", accountID[len(accountID)-4:])
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
