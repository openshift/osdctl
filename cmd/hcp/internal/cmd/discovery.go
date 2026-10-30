package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
)

const backupTimestampLayout = "20060102150405"

type DiscoveryRequest struct {
	ClusterID  string
	Limit      int
	AWSProfile string
}

type discoveredBackup struct {
	id        string
	timestamp time.Time
}

// BackupDiscoverer lists available HCP backups.
type BackupDiscoverer interface {
	DiscoverBackups(ctx context.Context, request DiscoveryRequest) ([]DiscoveredBackup, error)
}

// DiscoveredBackup identifies a backup and its creation timestamp.
type DiscoveredBackup struct {
	ID        string
	Timestamp time.Time
}

// DefaultBackupDiscoverer is the production backup discovery implementation.
type DefaultBackupDiscoverer struct {
	ocmClient discoveryOCMClient
	awsClient internalclient.AWSAccountClientProvider
}

type DefaultBackupDiscovererConfig struct {
	AWSClient internalclient.AWSAccountClientProvider
}

func (c *DefaultBackupDiscovererConfig) Options(opts ...DefaultBackupDiscovererOption) {
	for _, opt := range opts {
		opt.ConfigureDefaultBackupDiscoverer(c)
	}
}

func (c *DefaultBackupDiscovererConfig) Defaults() {
	if c.AWSClient == nil {
		c.AWSClient = internalclient.NewAWSAccountClientProvider()
	}
}

type DefaultBackupDiscovererOption interface {
	ConfigureDefaultBackupDiscoverer(*DefaultBackupDiscovererConfig)
}

type discoveryOCMClient interface {
	internalclient.ClusterResolver
	BackupConfig(ctx context.Context, clusterID, managementCluster string) (*cmv1.AWSBackupConfig, error)
}

func NewDefaultBackupDiscoverer(ocmClient discoveryOCMClient, opts ...DefaultBackupDiscovererOption) *DefaultBackupDiscoverer {
	var cfg DefaultBackupDiscovererConfig
	cfg.Options(opts...)
	cfg.Defaults()

	return &DefaultBackupDiscoverer{ocmClient: ocmClient, awsClient: cfg.AWSClient}
}

func (r *DefaultBackupDiscoverer) DiscoverBackups(ctx context.Context, request DiscoveryRequest) ([]DiscoveredBackup, error) {
	if request.Limit == 0 || request.Limit < -1 {
		return nil, fmt.Errorf("limit must be a positive number or -1 to display all backups")
	}

	cluster, err := r.ocmClient.Resolve(ctx, request.ClusterID)
	if err != nil {
		return nil, fmt.Errorf("resolving cluster: %w", err)
	}
	if cluster.HCPClusterRegion == "" {
		return nil, fmt.Errorf("HCP cluster %s has no AWS region", cluster.HCPClusterID)
	}

	backupConfig, err := r.ocmClient.BackupConfig(ctx, cluster.HCPClusterID, cluster.MgmtClusterName)
	if err != nil {
		return nil, err
	}

	s3Client, err := r.awsClient.AccountClient(request.AWSProfile, cluster.HCPClusterRegion, backupConfig.AccountId())
	if err != nil {
		return nil, fmt.Errorf("creating backup S3 client: %w", err)
	}

	backups, err := listBackups(s3Client, backupConfig.S3Bucket(), cluster.HCPClusterID)
	if err != nil {
		return nil, err
	}
	if len(backups) == 0 {
		return nil, fmt.Errorf("no valid backups found for cluster %s", cluster.HCPClusterID)
	}

	limit := request.Limit
	if limit == -1 || limit > len(backups) {
		limit = len(backups)
	}
	results := make([]DiscoveredBackup, 0, limit)
	for _, backup := range backups[:limit] {
		results = append(results, DiscoveredBackup{ID: backup.id, Timestamp: backup.timestamp})
	}
	return results, nil
}

func listBackups(client awsprovider.Client, bucket, clusterID string) ([]discoveredBackup, error) {
	prefix := clusterID + "/backups/"
	backups := make(map[string]discoveredBackup)
	var continuationToken *string
	var commonPrefixCount, objectCount int
	for {
		output, err := client.ListObjectsV2(&s3.ListObjectsV2Input{
			Bucket:            awssdk.String(bucket),
			Prefix:            awssdk.String(prefix),
			Delimiter:         awssdk.String("/"),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("listing backups in s3://%s/%s: %w", bucket, prefix, err)
		}
		if output == nil {
			return nil, fmt.Errorf("listing backups in s3://%s/%s returned no result", bucket, prefix)
		}
		for _, commonPrefix := range output.CommonPrefixes {
			commonPrefixCount++
			if commonPrefix.Prefix == nil {
				continue
			}
			backup, ok := parseBackupPrefix(prefix, *commonPrefix.Prefix)
			if ok {
				backups[backup.id] = backup
			}
		}
		for _, object := range output.Contents {
			objectCount++
			if object.Key == nil {
				continue
			}
			backup, ok := parseBackupKey(prefix, *object.Key)
			if ok {
				backups[backup.id] = backup
			}
		}

		if !awssdk.ToBool(output.IsTruncated) {
			break
		}
		if output.NextContinuationToken == nil || *output.NextContinuationToken == "" {
			return nil, fmt.Errorf("listing backups in s3://%s/%s returned no continuation token", bucket, prefix)
		}
		continuationToken = output.NextContinuationToken
	}

	if len(backups) == 0 {
		return nil, fmt.Errorf("no valid backup prefixes found in s3://%s/%s (%d common prefixes, %d objects)", bucket, prefix, commonPrefixCount, objectCount)
	}

	result := make([]discoveredBackup, 0, len(backups))
	for _, backup := range backups {
		result = append(result, backup)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].timestamp.Equal(result[j].timestamp) {
			return result[i].id > result[j].id
		}
		return result[i].timestamp.After(result[j].timestamp)
	})

	return result, nil
}

func parseBackupPrefix(parentPrefix, prefix string) (discoveredBackup, bool) {
	if !strings.HasPrefix(prefix, parentPrefix) {
		return discoveredBackup{}, false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(prefix, parentPrefix), "/")
	if id == "" || strings.Contains(id, "/") {
		return discoveredBackup{}, false
	}

	timestampStart := strings.LastIndex(id, "-") + 1
	if timestampStart == 0 {
		return discoveredBackup{}, false
	}
	timestamp, err := time.Parse(backupTimestampLayout, id[timestampStart:])
	if err != nil {
		return discoveredBackup{}, false
	}

	return discoveredBackup{id: id, timestamp: timestamp}, true
}

func parseBackupKey(parentPrefix, key string) (discoveredBackup, bool) {
	if !strings.HasPrefix(key, parentPrefix) {
		return discoveredBackup{}, false
	}
	backupID, _, _ := strings.Cut(strings.TrimPrefix(key, parentPrefix), "/")
	return parseBackupPrefix(parentPrefix, parentPrefix+backupID+"/")
}
