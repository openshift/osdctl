package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	logrus "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	snapshotJournalVersion = 2
)

func snapshotJournalName(clusterID string) string {
	return fmt.Sprintf("snapshot-copy-journal-%s.json", clusterID)
}

// CopySnapshotsRequest identifies an HCP snapshot-copy operation.
type CopySnapshotsRequest struct {
	ClusterID           string
	SourceMC            string
	ReplacementMC       string
	AWSProfile          string
	SnapshotJournalPath string
	Reason              string
	Timeout             time.Duration
	Concurrency         int
}

// SnapshotCopier prepares and executes an HCP snapshot-copy operation.
type SnapshotCopier interface {
	PrepareSnapshotCopy(context.Context, CopySnapshotsRequest) (SnapshotCopyPlan, error)
	CopySnapshots(context.Context, SnapshotCopyPlan) error
}

// SnapshotCopyPlan contains the validated inputs and clients for a copy operation.
type SnapshotCopyPlan struct {
	ClusterID                    string
	Region                       string
	SnapshotJournalPath          string
	SourceAccountID              string
	ReplacementAccountID         string
	SourceManagementCluster      string
	ReplacementManagementCluster string
	Entries                      []SnapshotCopyEntry
	VSCInventory                 []map[string]any
	Concurrency                  int

	source      awsprovider.Client
	destination awsprovider.Client
}

// SnapshotCopyEntry maps one VolumeSnapshotContent to its copied snapshots.
type SnapshotCopyEntry struct {
	VSCName                string `json:"vscName"`
	OriginalSnapshotID     string `json:"originalSnapshotID"`
	IntermediarySnapshotID string `json:"intermediarySnapshotID"`
	DestinationSnapshotID  string `json:"destinationSnapshotID"`
	IntermediaryDeleted    bool   `json:"intermediaryDeleted"`
}

type snapshotCopyJournal struct {
	Version                    int `json:"version"`
	ClusterID                  string
	Region                     string
	SourceAccountID            string
	ReplacementAccountID       string
	SourceCMKID                string
	SourceCMKAlias             string
	SourceCMKGrantID           string
	SourceCMKAliasCreated      bool
	SourceCMKGrantRevoked      bool
	SourceCMKAliasDeleted      bool
	SourceCMKDeletionScheduled bool
	OperationID                string
	Entries                    []SnapshotCopyEntry `json:"entries"`
	VSCInventory               []map[string]any    `json:"vscInventory"`
}

// DefaultSnapshotCopier performs cross-account EBS snapshot copies.
type DefaultSnapshotCopier struct {
	ocmClient snapshotCopyOCMClient
	awsClient internalclient.AWSAccountClientProvider
	now       func() time.Time
	logger    *logrus.Logger
}

type snapshotCopyOCMClient interface {
	internalclient.ClusterResolver
	Cluster(string) (*cmv1.Cluster, error)
	HCPNamespace(context.Context, string) (string, error)
	ElevatedManagementCluster(string, string) (internalclient.ManagementClusterClient, error)
}

// NewDefaultSnapshotCopier creates a snapshot copier with a default logger.
func NewDefaultSnapshotCopier(ocmClient snapshotCopyOCMClient) *DefaultSnapshotCopier {
	return NewDefaultSnapshotCopierWithLogger(ocmClient, logrus.New())
}

// NewDefaultSnapshotCopierWithLogger creates a snapshot copier that reports progress through logger.
func NewDefaultSnapshotCopierWithLogger(ocmClient snapshotCopyOCMClient, logger *logrus.Logger) *DefaultSnapshotCopier {
	if logger == nil {
		logger = logrus.New()
	}
	return &DefaultSnapshotCopier{ocmClient: ocmClient, awsClient: internalclient.NewAWSAccountClientProvider(), now: time.Now, logger: logger}
}

func (c *DefaultSnapshotCopier) PrepareSnapshotCopy(ctx context.Context, request CopySnapshotsRequest) (SnapshotCopyPlan, error) {
	if err := validateCopySnapshotsRequest(request); err != nil {
		return SnapshotCopyPlan{}, err
	}
	cluster, err := c.ocmClient.Resolve(ctx, request.ClusterID)
	if err != nil {
		return SnapshotCopyPlan{}, fmt.Errorf("resolving cluster: %w", err)
	}
	if cluster.HCPClusterRegion == "" {
		return SnapshotCopyPlan{}, fmt.Errorf("HCP cluster %s has no AWS region", cluster.HCPClusterID)
	}
	journalPath, err := resolveSnapshotJournalPath(request.SnapshotJournalPath, cluster.HCPClusterID)
	if err != nil {
		return SnapshotCopyPlan{}, err
	}
	source, err := c.ocmClient.Cluster(request.SourceMC)
	if err != nil {
		return SnapshotCopyPlan{}, fmt.Errorf("resolving source management cluster: %w", err)
	}
	replacement, err := c.ocmClient.Cluster(request.ReplacementMC)
	if err != nil {
		return SnapshotCopyPlan{}, fmt.Errorf("resolving replacement management cluster: %w", err)
	}
	if source.ID() == replacement.ID() {
		return SnapshotCopyPlan{}, fmt.Errorf("source and replacement management clusters must differ")
	}
	for name, managementCluster := range map[string]*cmv1.Cluster{"source": source, "replacement": replacement} {
		if managementCluster.ID() == "" || managementCluster.Name() == "" {
			return SnapshotCopyPlan{}, fmt.Errorf("%s management cluster is incomplete", name)
		}
		if managementCluster.Region() == nil || managementCluster.Region().ID() != cluster.HCPClusterRegion {
			return SnapshotCopyPlan{}, fmt.Errorf("%s management cluster is not in HCP region %s", name, cluster.HCPClusterRegion)
		}
	}
	sourceAccountID, err := managementClusterAccountID(source)
	if err != nil {
		return SnapshotCopyPlan{}, fmt.Errorf("getting source management cluster account: %w", err)
	}
	replacementAccountID, err := managementClusterAccountID(replacement)
	if err != nil {
		return SnapshotCopyPlan{}, fmt.Errorf("getting replacement management cluster account: %w", err)
	}

	managementClient, err := c.ocmClient.ElevatedManagementCluster(replacement.ID(), request.Reason)
	if err != nil {
		return SnapshotCopyPlan{}, fmt.Errorf("connecting to replacement management cluster: %w", err)
	}
	hcpNamespace, err := c.ocmClient.HCPNamespace(ctx, cluster.HCPClusterID)
	if err != nil {
		return SnapshotCopyPlan{}, fmt.Errorf("getting HCP namespace: %w", err)
	}
	contents, err := managementClient.ListVolumeSnapshotContents(ctx)
	if err != nil {
		return SnapshotCopyPlan{}, err
	}
	entries, err := snapshotEntries(contents, hcpNamespace)
	if err != nil {
		return SnapshotCopyPlan{}, err
	}

	sourceClient, err := c.awsClient.AccountClient(request.AWSProfile, cluster.HCPClusterRegion, sourceAccountID)
	if err != nil {
		return SnapshotCopyPlan{}, fmt.Errorf("creating source account client: %w", err)
	}
	destinationClient, err := c.awsClient.AccountClient(request.AWSProfile, cluster.HCPClusterRegion, replacementAccountID)
	if err != nil {
		return SnapshotCopyPlan{}, fmt.Errorf("creating replacement account client: %w", err)
	}
	if err := validateAccount(sourceClient, sourceAccountID); err != nil {
		return SnapshotCopyPlan{}, fmt.Errorf("validating source account: %w", err)
	}
	if err := validateAccount(destinationClient, replacementAccountID); err != nil {
		return SnapshotCopyPlan{}, fmt.Errorf("validating replacement account: %w", err)
	}
	for _, entry := range entries {
		if err := validateSourceSnapshot(ctx, sourceClient, entry.OriginalSnapshotID); err != nil {
			return SnapshotCopyPlan{}, err
		}
	}

	inventory := selectedVSCInventory(contents, entries)
	return SnapshotCopyPlan{ClusterID: cluster.HCPClusterID, Region: cluster.HCPClusterRegion, SnapshotJournalPath: journalPath, SourceAccountID: sourceAccountID, ReplacementAccountID: replacementAccountID, SourceManagementCluster: source.Name(), ReplacementManagementCluster: replacement.Name(), Entries: entries, VSCInventory: inventory, Concurrency: request.Concurrency, source: sourceClient, destination: destinationClient}, nil
}

func (c *DefaultSnapshotCopier) CopySnapshots(ctx context.Context, plan SnapshotCopyPlan) error {
	if plan.Concurrency <= 0 {
		return fmt.Errorf("concurrency must be greater than zero")
	}
	journal, err := loadOrCreateSnapshotJournal(plan.SnapshotJournalPath, plan)
	if err != nil {
		return err
	}
	if journal.SourceCMKID == "" {
		alias := fmt.Sprintf("alias/hcp-migration-%s-%s", plan.ClusterID, c.now().UTC().Format("20060102150405"))
		c.logger.Info("Creating temporary source KMS key for snapshot copies")
		key, err := plan.source.CreateKey(ctx, &kms.CreateKeyInput{Description: awssdk.String("Temporary HCP recovery snapshot migration key")})
		if err != nil {
			return fmt.Errorf("creating source KMS key: %w", err)
		}
		if key == nil || key.KeyMetadata == nil || key.KeyMetadata.KeyId == nil {
			return fmt.Errorf("creating source KMS key returned no key ID")
		}
		journal.SourceCMKID, journal.SourceCMKAlias = *key.KeyMetadata.KeyId, alias
		if err := saveSnapshotJournal(plan.SnapshotJournalPath, journal); err != nil {
			return err
		}
		c.logger.Info("Created temporary source KMS key")
	} else {
		c.logger.Info("Reusing temporary source KMS key from snapshot journal")
	}
	if journal.SourceCMKAlias != "" && !journal.SourceCMKAliasCreated {
		c.logger.Info("Creating source KMS key alias")
		if _, err := plan.source.CreateAlias(ctx, &kms.CreateAliasInput{AliasName: awssdk.String(journal.SourceCMKAlias), TargetKeyId: awssdk.String(journal.SourceCMKID)}); err != nil {
			if !isAlreadyExists(err) {
				return fmt.Errorf("creating source KMS key alias: %w", err)
			}
		}
		journal.SourceCMKAliasCreated = true
		if err := saveSnapshotJournal(plan.SnapshotJournalPath, journal); err != nil {
			return err
		}
		c.logger.Info("Source KMS key alias is ready")
	}
	if journal.SourceCMKGrantID == "" {
		c.logger.Info("Granting replacement account access to source KMS key")
		identity, err := plan.destination.GetCallerIdentity(nil)
		if err != nil {
			return fmt.Errorf("getting replacement caller identity: %w", err)
		}
		principal, err := assumedRolePrincipal(awssdk.ToString(identity.Arn))
		if err != nil {
			return err
		}
		grant, err := plan.source.CreateGrant(ctx, &kms.CreateGrantInput{KeyId: awssdk.String(journal.SourceCMKID), GranteePrincipal: awssdk.String(principal), Operations: []kmstypes.GrantOperation{kmstypes.GrantOperationDecrypt, kmstypes.GrantOperationEncrypt, kmstypes.GrantOperationGenerateDataKeyWithoutPlaintext, kmstypes.GrantOperationReEncryptFrom, kmstypes.GrantOperationReEncryptTo, kmstypes.GrantOperationCreateGrant, kmstypes.GrantOperationDescribeKey}})
		if err != nil {
			return fmt.Errorf("granting replacement account access to source KMS key: %w", err)
		}
		if grant == nil || grant.GrantId == nil {
			return fmt.Errorf("granting replacement account access to source KMS key returned no grant ID")
		}
		journal.SourceCMKGrantID = *grant.GrantId
		if err := saveSnapshotJournal(plan.SnapshotJournalPath, journal); err != nil {
			return err
		}
		c.logger.Info("Replacement account KMS grant is ready")
	}
	writer := snapshotJournalWriter{path: plan.SnapshotJournalPath, journal: &journal}
	c.logger.WithField("snapshot_count", len(journal.Entries)).Info("Starting snapshot copy chains")
	if err := runBounded(ctx, len(journal.Entries), plan.Concurrency, func(ctx context.Context, index int) error {
		return copySnapshotEntry(ctx, plan, &writer, c.logger, index)
	}); err != nil {
		return err
	}
	c.logger.WithField("snapshot_count", len(journal.Entries)).Info("Snapshot copy chains completed")
	return cleanupSnapshotCopies(ctx, plan.source, &writer, c.logger)
}

type snapshotJournalWriter struct {
	path    string
	journal *snapshotCopyJournal
	mu      sync.Mutex
}

func (w *snapshotJournalWriter) entry(index int) SnapshotCopyEntry {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.journal.Entries[index]
}

func (w *snapshotJournalWriter) updateEntry(index int, update func(*SnapshotCopyEntry)) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	update(&w.journal.Entries[index])
	return saveSnapshotJournal(w.path, *w.journal)
}

func copySnapshotEntry(ctx context.Context, plan SnapshotCopyPlan, writer *snapshotJournalWriter, logger *logrus.Logger, index int) error {
	entry := writer.entry(index)
	fields := logrus.Fields{"copy": fmt.Sprintf("%d/%d", index+1, len(writer.journal.Entries))}
	logger.WithFields(fields).Info("Processing snapshot copy")
	if entry.IntermediaryDeleted {
		if entry.DestinationSnapshotID == "" {
			return fmt.Errorf("intermediary snapshot for VSC %s was deleted before a destination snapshot was recorded", entry.VSCName)
		}
		if err := validateDestinationSnapshot(ctx, plan.destination, entry.DestinationSnapshotID, plan.ReplacementAccountID); err != nil {
			return fmt.Errorf("validating destination snapshot for VSC %s: %w", entry.VSCName, err)
		}
		logger.WithFields(fields).Info("Snapshot copy was completed before cleanup")
		return nil
	}
	if entry.IntermediarySnapshotID == "" {
		id, err := findOperationSnapshot(ctx, plan.source, writer.journal.OperationID, entry.VSCName, "intermediary")
		if err != nil {
			return fmt.Errorf("finding intermediary snapshot for VSC %s: %w", entry.VSCName, err)
		}
		if id == "" {
			logger.WithFields(fields).Info("Creating intermediary snapshot copy")
			output, err := plan.source.CopySnapshot(ctx, &ec2.CopySnapshotInput{SourceRegion: awssdk.String(plan.Region), SourceSnapshotId: awssdk.String(entry.OriginalSnapshotID), Encrypted: awssdk.Bool(true), KmsKeyId: awssdk.String(writer.journal.SourceCMKID), TagSpecifications: snapshotCopyTags(plan.ClusterID, writer.journal.OperationID, entry.VSCName, "intermediary")})
			if err != nil {
				return fmt.Errorf("copying source snapshot %s for VSC %s: %w", entry.OriginalSnapshotID, entry.VSCName, err)
			}
			if output == nil || output.SnapshotId == nil {
				return fmt.Errorf("copying source snapshot %s for VSC %s returned no snapshot ID", entry.OriginalSnapshotID, entry.VSCName)
			}
			id = *output.SnapshotId
		} else {
			logger.WithFields(fields).Info("Resuming tagged intermediary snapshot copy")
		}
		if err := writer.updateEntry(index, func(entry *SnapshotCopyEntry) { entry.IntermediarySnapshotID = id }); err != nil {
			return err
		}
		entry.IntermediarySnapshotID = id
	} else {
		logger.WithFields(fields).Info("Reusing intermediary snapshot from journal")
	}
	logger.WithFields(fields).Info("Waiting for intermediary snapshot copy")
	if err := waitForSnapshot(ctx, plan.source, entry.IntermediarySnapshotID, 5*time.Second); err != nil {
		return fmt.Errorf("waiting for intermediary snapshot for VSC %s: %w", entry.VSCName, err)
	}
	logger.WithFields(fields).Info("Sharing intermediary snapshot with replacement account")
	if _, err := plan.source.ModifySnapshotAttribute(ctx, &ec2.ModifySnapshotAttributeInput{SnapshotId: awssdk.String(entry.IntermediarySnapshotID), Attribute: ec2types.SnapshotAttributeNameCreateVolumePermission, OperationType: ec2types.OperationTypeAdd, UserIds: []string{plan.ReplacementAccountID}}); err != nil {
		return fmt.Errorf("sharing intermediary snapshot %s for VSC %s: %w", entry.IntermediarySnapshotID, entry.VSCName, err)
	}
	if entry.DestinationSnapshotID == "" {
		id, err := findOperationSnapshot(ctx, plan.destination, writer.journal.OperationID, entry.VSCName, "destination")
		if err != nil {
			return fmt.Errorf("finding destination snapshot for VSC %s: %w", entry.VSCName, err)
		}
		if id == "" {
			logger.WithFields(fields).Info("Creating destination snapshot copy")
			output, err := plan.destination.CopySnapshot(ctx, &ec2.CopySnapshotInput{SourceRegion: awssdk.String(plan.Region), SourceSnapshotId: awssdk.String(entry.IntermediarySnapshotID), Encrypted: awssdk.Bool(true), TagSpecifications: snapshotCopyTags(plan.ClusterID, writer.journal.OperationID, entry.VSCName, "destination")})
			if err != nil {
				return fmt.Errorf("copying intermediary snapshot %s for VSC %s: %w", entry.IntermediarySnapshotID, entry.VSCName, err)
			}
			if output == nil || output.SnapshotId == nil {
				return fmt.Errorf("copying intermediary snapshot %s for VSC %s returned no snapshot ID", entry.IntermediarySnapshotID, entry.VSCName)
			}
			id = *output.SnapshotId
		} else {
			logger.WithFields(fields).Info("Resuming tagged destination snapshot copy")
		}
		if err := writer.updateEntry(index, func(entry *SnapshotCopyEntry) { entry.DestinationSnapshotID = id }); err != nil {
			return err
		}
		entry.DestinationSnapshotID = id
	} else {
		logger.WithFields(fields).Info("Reusing destination snapshot from journal")
	}
	logger.WithFields(fields).Info("Waiting for destination snapshot copy")
	if err := waitForSnapshot(ctx, plan.destination, entry.DestinationSnapshotID, 5*time.Second); err != nil {
		return fmt.Errorf("waiting for destination snapshot for VSC %s: %w", entry.VSCName, err)
	}
	if err := validateDestinationSnapshot(ctx, plan.destination, entry.DestinationSnapshotID, plan.ReplacementAccountID); err != nil {
		return fmt.Errorf("validating destination snapshot for VSC %s: %w", entry.VSCName, err)
	}
	logger.WithFields(fields).Info("Snapshot copy completed")
	return nil
}

func cleanupSnapshotCopies(ctx context.Context, source awsprovider.Client, writer *snapshotJournalWriter, logger *logrus.Logger) error {
	logger.WithField("snapshot_count", len(writer.journal.Entries)).Info("Cleaning up intermediary snapshots and temporary KMS key")
	for index := range writer.journal.Entries {
		entry := writer.entry(index)
		if entry.IntermediaryDeleted {
			continue
		}
		if entry.IntermediarySnapshotID == "" {
			return fmt.Errorf("intermediary snapshot for VSC %s is missing", entry.VSCName)
		}
		logger.WithField("copy", fmt.Sprintf("%d/%d", index+1, len(writer.journal.Entries))).Info("Deleting intermediary snapshot")
		if _, err := source.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: awssdk.String(entry.IntermediarySnapshotID)}); err != nil && !isAPIError(err, "InvalidSnapshot.NotFound") {
			return fmt.Errorf("deleting intermediary snapshot %s for VSC %s: %w", entry.IntermediarySnapshotID, entry.VSCName, err)
		}
		if err := writer.updateEntry(index, func(entry *SnapshotCopyEntry) { entry.IntermediaryDeleted = true }); err != nil {
			return err
		}
	}

	journal := writer.journal
	if !journal.SourceCMKGrantRevoked && journal.SourceCMKGrantID != "" {
		logger.Info("Revoking temporary KMS grant")
		if _, err := source.RevokeGrant(ctx, &kms.RevokeGrantInput{KeyId: awssdk.String(journal.SourceCMKID), GrantId: awssdk.String(journal.SourceCMKGrantID)}); err != nil && !isAPIError(err, "NotFoundException") {
			return fmt.Errorf("revoking temporary KMS grant: %w", err)
		}
		journal.SourceCMKGrantRevoked = true
		if err := saveSnapshotJournal(writer.path, *journal); err != nil {
			return err
		}
	}
	if !journal.SourceCMKAliasDeleted && journal.SourceCMKAlias != "" {
		logger.Info("Deleting temporary KMS key alias")
		if _, err := source.DeleteAlias(ctx, &kms.DeleteAliasInput{AliasName: awssdk.String(journal.SourceCMKAlias)}); err != nil && !isAPIError(err, "NotFoundException") {
			return fmt.Errorf("deleting temporary KMS key alias: %w", err)
		}
		journal.SourceCMKAliasDeleted = true
		if err := saveSnapshotJournal(writer.path, *journal); err != nil {
			return err
		}
	}
	if journal.SourceCMKDeletionScheduled {
		return nil
	}
	logger.Info("Scheduling temporary KMS key deletion")
	if _, err := source.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{KeyId: awssdk.String(journal.SourceCMKID), PendingWindowInDays: awssdk.Int32(7)}); err != nil {
		if !isAPIError(err, "KMSInvalidStateException") {
			return fmt.Errorf("scheduling temporary KMS key deletion: %w", err)
		}
	}
	key, err := source.DescribeKey(ctx, &kms.DescribeKeyInput{KeyId: awssdk.String(journal.SourceCMKID)})
	if err != nil {
		return fmt.Errorf("describing temporary KMS key after scheduling deletion: %w", err)
	}
	if key == nil || key.KeyMetadata == nil || key.KeyMetadata.KeyState != kmstypes.KeyStatePendingDeletion {
		return fmt.Errorf("temporary KMS key %s is not pending deletion", journal.SourceCMKID)
	}
	journal.SourceCMKDeletionScheduled = true
	if err := saveSnapshotJournal(writer.path, *journal); err != nil {
		return err
	}
	logger.Info("Temporary KMS key is pending deletion")
	return nil
}

func runBounded(ctx context.Context, count, concurrency int, work func(context.Context, int) error) error {
	group, workCtx := errgroup.WithContext(ctx)
	jobs := make(chan int)
	for range concurrency {
		group.Go(func() error {
			for index := range jobs {
				if err := work(workCtx, index); err != nil {
					return err
				}
			}
			return nil
		})
	}
	group.Go(func() error {
		defer close(jobs)
		for index := range count {
			select {
			case jobs <- index:
			case <-workCtx.Done():
				return nil
			}
		}
		return nil
	})
	return group.Wait()
}

func isAlreadyExists(err error) bool {
	var apiError smithy.APIError
	return errors.As(err, &apiError) && apiError.ErrorCode() == "AlreadyExistsException"
}

func isAPIError(err error, code string) bool {
	var apiError smithy.APIError
	return errors.As(err, &apiError) && apiError.ErrorCode() == code
}

func snapshotCopyTags(clusterID, operationID, vscName, stage string) []ec2types.TagSpecification {
	return []ec2types.TagSpecification{{ResourceType: ec2types.ResourceTypeSnapshot, Tags: []ec2types.Tag{{Key: awssdk.String("Name"), Value: awssdk.String(snapshotCopyName(clusterID, vscName, stage))}, {Key: awssdk.String("osdctl.openshift.io/snapshot-copy"), Value: awssdk.String(operationID)}, {Key: awssdk.String("osdctl.openshift.io/vsc"), Value: awssdk.String(vscName)}, {Key: awssdk.String("osdctl.openshift.io/stage"), Value: awssdk.String(stage)}}}}
}

func snapshotCopyName(clusterID, vscName, stage string) string {
	const maxLength = 256
	const basePrefix = "osdctl-hcp-"
	suffix := "-" + stage
	maxClusterIDLength := maxLength - len(basePrefix) - len(suffix)
	if len(clusterID) > maxClusterIDLength {
		return basePrefix + clusterID[:maxClusterIDLength] + suffix
	}
	prefix := fmt.Sprintf("%s%s-", basePrefix, clusterID)
	maxVSCNameLength := maxLength - len(prefix) - len(suffix)
	if len(vscName) > maxVSCNameLength {
		vscName = vscName[:maxVSCNameLength]
	}
	return prefix + vscName + suffix
}

func findOperationSnapshot(ctx context.Context, client awsprovider.Client, operationID, vscName, stage string) (string, error) {
	output, err := client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{OwnerIds: []string{"self"}, Filters: []ec2types.Filter{{Name: awssdk.String("tag:osdctl.openshift.io/snapshot-copy"), Values: []string{operationID}}, {Name: awssdk.String("tag:osdctl.openshift.io/vsc"), Values: []string{vscName}}, {Name: awssdk.String("tag:osdctl.openshift.io/stage"), Values: []string{stage}}}})
	if err != nil {
		return "", fmt.Errorf("finding %s snapshot for VSC %s: %w", stage, vscName, err)
	}
	if output == nil || len(output.Snapshots) == 0 {
		return "", nil
	}
	if len(output.Snapshots) != 1 || output.Snapshots[0].SnapshotId == nil {
		return "", fmt.Errorf("multiple %s snapshots found for VSC %s", stage, vscName)
	}
	return *output.Snapshots[0].SnapshotId, nil
}

func validateCopySnapshotsRequest(r CopySnapshotsRequest) error {
	for name, value := range map[string]string{"cluster ID": r.ClusterID, "source management cluster": r.SourceMC, "replacement management cluster": r.ReplacementMC, "reason": r.Reason} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if r.Timeout <= 0 {
		return fmt.Errorf("timeout must be greater than zero")
	}
	if r.Concurrency <= 0 {
		return fmt.Errorf("concurrency must be greater than zero")
	}
	return nil
}

func managementClusterAccountID(cluster *cmv1.Cluster) (string, error) {
	if cluster == nil || cluster.AWS() == nil || cluster.AWS().STS() == nil {
		return "", fmt.Errorf("management cluster has no AWS STS support role")
	}
	supportRoleARN := cluster.AWS().STS().SupportRoleARN()
	parsed, err := arn.Parse(supportRoleARN)
	if err != nil || parsed.Service != "iam" || parsed.AccountID == "" {
		return "", fmt.Errorf("invalid AWS STS support role ARN %q", supportRoleARN)
	}
	return parsed.AccountID, nil
}

func resolveSnapshotJournalPath(selected, clusterID string) (string, error) {
	if strings.TrimSpace(selected) != "" {
		return filepath.Clean(selected), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return defaultSnapshotJournalPath(os.Getenv("XDG_DATA_HOME"), home, clusterID)
}

func defaultSnapshotJournalPath(xdgDataHome, home, clusterID string) (string, error) {
	journalName := snapshotJournalName(clusterID)
	candidates := make([]string, 0, 2)
	if xdgDataHome != "" {
		candidates = append(candidates, filepath.Join(xdgDataHome, "osdctl", journalName))
	}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, ".local", "share", "osdctl", journalName))
	}
	for _, candidate := range candidates {
		if err := os.MkdirAll(filepath.Dir(candidate), 0700); err == nil {
			return candidate, nil
		}
	}
	return journalName, nil
}

func snapshotEntries(contents *unstructured.UnstructuredList, hcpNamespace string) ([]SnapshotCopyEntry, error) {
	entries := make([]SnapshotCopyEntry, 0)
	for _, content := range contents.Items {
		namespace, _, _ := unstructured.NestedString(content.Object, "spec", "volumeSnapshotRef", "namespace")
		if namespace != hcpNamespace {
			continue
		}
		driver, _, _ := unstructured.NestedString(content.Object, "spec", "driver")
		handle, _, _ := unstructured.NestedString(content.Object, "status", "snapshotHandle")
		if handle == "" {
			handle, _, _ = unstructured.NestedString(content.Object, "spec", "source", "snapshotHandle")
		}
		if driver != "ebs.csi.aws.com" || !strings.HasPrefix(handle, "snap-") {
			continue
		}
		entries = append(entries, SnapshotCopyEntry{VSCName: content.GetName(), OriginalSnapshotID: handle})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no EBS VolumeSnapshotContents found in HCP namespace %s", hcpNamespace)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].VSCName < entries[j].VSCName })
	return entries, nil
}

func selectedVSCInventory(contents *unstructured.UnstructuredList, entries []SnapshotCopyEntry) []map[string]any {
	selected := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		selected[entry.VSCName] = struct{}{}
	}
	inventory := make([]map[string]any, 0, len(entries))
	for _, content := range contents.Items {
		if _, ok := selected[content.GetName()]; ok {
			inventory = append(inventory, content.Object)
		}
	}
	return inventory
}

func validateAccount(client awsprovider.Client, expected string) error {
	identity, err := client.GetCallerIdentity(nil)
	if err != nil {
		return err
	}
	if awssdk.ToString(identity.Account) != expected {
		return fmt.Errorf("authenticated to account %s, expected %s", awssdk.ToString(identity.Account), expected)
	}
	return nil
}
func validateSourceSnapshot(ctx context.Context, client awsprovider.Client, id string) error {
	output, err := client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{SnapshotIds: []string{id}})
	if err != nil {
		return fmt.Errorf("describing source snapshot %s: %w", id, err)
	}
	if len(output.Snapshots) != 1 || output.Snapshots[0].State != ec2types.SnapshotStateCompleted {
		return fmt.Errorf("source snapshot %s is not completed", id)
	}
	return nil
}
func validateDestinationSnapshot(ctx context.Context, client awsprovider.Client, id, account string) error {
	output, err := client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{SnapshotIds: []string{id}})
	if err != nil {
		return fmt.Errorf("describing destination snapshot %s: %w", id, err)
	}
	if len(output.Snapshots) != 1 || output.Snapshots[0].OwnerId == nil || *output.Snapshots[0].OwnerId != account || !awssdk.ToBool(output.Snapshots[0].Encrypted) {
		return fmt.Errorf("destination snapshot %s is not owned and encrypted by account %s", id, account)
	}
	return nil
}
func waitForSnapshot(ctx context.Context, client awsprovider.Client, id string, interval time.Duration) error {
	for {
		output, err := client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{SnapshotIds: []string{id}})
		switch {
		case err != nil && !isAPIError(err, "InvalidSnapshot.NotFound"):
			return fmt.Errorf("describing snapshot %s: %w", id, err)
		case err == nil && (output == nil || len(output.Snapshots) == 0):
		case err == nil && len(output.Snapshots) != 1:
			return fmt.Errorf("snapshot %s matched multiple snapshots", id)
		case err == nil:
			switch output.Snapshots[0].State {
			case ec2types.SnapshotStateCompleted:
				return nil
			case ec2types.SnapshotStateError:
				return fmt.Errorf("snapshot %s entered error state", id)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for snapshot %s: %w", id, ctx.Err())
		case <-time.After(interval):
		}
	}
}
func assumedRolePrincipal(callerARN string) (string, error) {
	parsed, err := arn.Parse(callerARN)
	if err != nil || parsed.Service != "sts" || !strings.HasPrefix(parsed.Resource, "assumed-role/") {
		return "", fmt.Errorf("replacement caller ARN %q is not an assumed-role ARN", callerARN)
	}
	roleAndSession := strings.TrimPrefix(parsed.Resource, "assumed-role/")
	roleName, sessionName, ok := strings.Cut(roleAndSession, "/")
	if !ok || roleName == "" || sessionName == "" {
		return "", fmt.Errorf("replacement caller ARN %q has no role", callerARN)
	}
	return fmt.Sprintf("arn:%s:iam::%s:role/%s", parsed.Partition, parsed.AccountID, roleName), nil
}
func loadOrCreateSnapshotJournal(path string, plan SnapshotCopyPlan) (snapshotCopyJournal, error) {
	var journal snapshotCopyJournal
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		journal = snapshotCopyJournal{Version: snapshotJournalVersion, OperationID: uuid.NewString(), ClusterID: plan.ClusterID, Region: plan.Region, SourceAccountID: plan.SourceAccountID, ReplacementAccountID: plan.ReplacementAccountID, Entries: plan.Entries, VSCInventory: plan.VSCInventory}
		return journal, saveSnapshotJournal(path, journal)
	}
	if err != nil {
		return journal, fmt.Errorf("reading snapshot journal: %w", err)
	}
	if err := json.Unmarshal(data, &journal); err != nil {
		return journal, fmt.Errorf("parsing snapshot journal: %w", err)
	}
	if journal.Version != snapshotJournalVersion || journal.OperationID == "" || journal.ClusterID != plan.ClusterID || journal.Region != plan.Region || journal.SourceAccountID != plan.SourceAccountID || journal.ReplacementAccountID != plan.ReplacementAccountID || !sameSnapshotSources(journal.Entries, plan.Entries) {
		return journal, fmt.Errorf("snapshot journal does not match this operation")
	}
	return journal, nil
}

func sameSnapshotSources(journaled, discovered []SnapshotCopyEntry) bool {
	if len(journaled) != len(discovered) {
		return false
	}
	sources := make(map[string]string, len(journaled))
	for _, entry := range journaled {
		if _, duplicate := sources[entry.VSCName]; duplicate {
			return false
		}
		sources[entry.VSCName] = entry.OriginalSnapshotID
	}
	for _, entry := range discovered {
		if sources[entry.VSCName] != entry.OriginalSnapshotID {
			return false
		}
	}
	return true
}
func saveSnapshotJournal(path string, journal snapshotCopyJournal) error {
	return writeJSON(path, journal)
}
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
