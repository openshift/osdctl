package backup

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/osdCloud"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const backupTimestampLayout = "20060102150405"

type discoveryFlags struct {
	clusterID  string
	limit      int
	awsProfile string
}

type discoveredBackup struct {
	id        string
	timestamp time.Time
}

type backupDiscoveryRunner struct {
	ocmConn             *ocmsdk.Connection
	resolver            ClusterResolver
	getBackupConfig     func(ctx context.Context, clusterID, managementCluster string) (*cmv1.AWSBackupConfig, error)
	newAWSClient        func(profile, region, configFile string) (awsprovider.Client, error)
	newAWSClientInput   func(input *awsprovider.ClientInput) (awsprovider.Client, error)
	generateSessionName func(client awsprovider.Client) (string, error)
	getAWSPartition     func(client awsprovider.Client) (string, error)
	generateCredentials func(client awsprovider.Client, accountID, sessionName, partition string) (*ststypes.Credentials, error)
	printer             Printer
}

func newCmdDiscover() *cobra.Command {
	flags := &discoveryFlags{}
	cmd := &cobra.Command{
		Use:   "discover --cluster-id <cluster-id>",
		Short: "Discover HCP backups from the disaster recovery S3 bucket",
		Example: "  osdctl hcp backup discover --cluster-id ${CLUSTER_ID}\n" +
			"  osdctl hcp backup discover --cluster-id ${CLUSTER_ID} --limit 5\n" +
			"  osdctl hcp backup discover --cluster-id ${CLUSTER_ID} --limit -1\n" +
			"  osdctl hcp backup discover --cluster-id ${CLUSTER_ID} --profile ${AWS_PROFILE}",
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

			runner := newBackupDiscoveryRunner(ocmConn, logger, &defaultPrinter{w: cmd.OutOrStdout()})
			return runner.run(cmd.Context(), *flags)
		},
	}

	cmd.Flags().StringVarP(&flags.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().IntVarP(&flags.limit, "limit", "l", 1, "Number of most recent backups to display (-1 displays all)")
	cmd.Flags().StringVarP(&flags.awsProfile, "profile", "p", "", "AWS profile used to assume the DR account role")
	_ = cmd.MarkFlagRequired("cluster-id")

	return cmd
}

func newBackupDiscoveryRunner(ocmConn *ocmsdk.Connection, logger *logrus.Logger, printer Printer) *backupDiscoveryRunner {
	runner := &backupDiscoveryRunner{
		ocmConn:             ocmConn,
		resolver:            &ocmClusterResolver{ocmConn: ocmConn, logger: logger},
		newAWSClient:        awsprovider.NewAwsClient,
		newAWSClientInput:   awsprovider.NewAwsClientWithInput,
		generateSessionName: osdCloud.GenerateRoleSessionName,
		getAWSPartition:     awsprovider.GetAwsPartition,
		generateCredentials: osdCloud.GenerateOrganizationAccountAccessCredentials,
		printer:             printer,
	}
	runner.getBackupConfig = func(ctx context.Context, clusterID, managementCluster string) (*cmv1.AWSBackupConfig, error) {
		return getBackupConfig(ctx, ocmConn, clusterID, managementCluster)
	}
	return runner
}

func (r *backupDiscoveryRunner) run(ctx context.Context, flags discoveryFlags) error {
	if flags.limit == 0 || flags.limit < -1 {
		return fmt.Errorf("limit must be a positive number or -1 to display all backups")
	}

	cluster, err := r.resolver.Resolve(ctx, flags.clusterID)
	if err != nil {
		return fmt.Errorf("resolving cluster: %w", err)
	}
	if cluster.HCPClusterRegion == "" {
		return fmt.Errorf("HCP cluster %s has no AWS region", cluster.HCPClusterID)
	}

	backupConfig, err := r.getBackupConfig(ctx, cluster.HCPClusterID, cluster.MgmtClusterName)
	if err != nil {
		return err
	}

	baseClient, err := r.newAWSClient(flags.awsProfile, cluster.HCPClusterRegion, "")
	if err != nil {
		return fmt.Errorf("creating AWS client: %w", err)
	}
	sessionName, err := r.generateSessionName(baseClient)
	if err != nil {
		return fmt.Errorf("generating AWS role session name: %w", err)
	}
	partition, err := r.getAWSPartition(baseClient)
	if err != nil {
		return fmt.Errorf("getting AWS partition: %w", err)
	}
	credentials, err := r.generateCredentials(baseClient, backupConfig.AccountId(), sessionName, partition)
	if err != nil {
		return fmt.Errorf("assuming DR account role: %w", err)
	}
	if credentials == nil || credentials.AccessKeyId == nil || credentials.SecretAccessKey == nil || credentials.SessionToken == nil {
		return fmt.Errorf("assuming DR account role returned incomplete credentials")
	}
	s3Client, err := r.newAWSClientInput(&awsprovider.ClientInput{
		AccessKeyID:     *credentials.AccessKeyId,
		SecretAccessKey: *credentials.SecretAccessKey,
		SessionToken:    *credentials.SessionToken,
		Region:          cluster.HCPClusterRegion,
	})
	if err != nil {
		return fmt.Errorf("creating backup S3 client: %w", err)
	}

	backups, err := listBackups(s3Client, backupConfig.S3Bucket(), cluster.HCPClusterID)
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		return fmt.Errorf("no valid backups found for cluster %s", cluster.HCPClusterID)
	}

	limit := flags.limit
	if limit == -1 || limit > len(backups) {
		limit = len(backups)
	}
	for _, backup := range backups[:limit] {
		r.printer.Printf("%s\n", backup.id)
	}

	return nil
}

func getBackupConfig(ctx context.Context, ocmConn *ocmsdk.Connection, clusterID, managementCluster string) (*cmv1.AWSBackupConfig, error) {
	if managementCluster == "" {
		return nil, fmt.Errorf("no management cluster found for %s", clusterID)
	}

	response, err := ocmConn.ClustersMgmt().V1().Clusters().Cluster(clusterID).ProvisionShard().Get().SendContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting provision shard for cluster %s: %w", clusterID, err)
	}
	if response == nil || response.Body() == nil || response.Body().HypershiftConfig() == nil || response.Body().HypershiftConfig().AWSShard() == nil {
		return nil, fmt.Errorf("no AWS backup configuration found for cluster %s", clusterID)
	}

	return selectBackupConfig(response.Body().HypershiftConfig().AWSShard().BackupConfigs(), managementCluster)
}

func selectBackupConfig(configs []*cmv1.AWSBackupConfig, managementCluster string) (*cmv1.AWSBackupConfig, error) {
	var matched *cmv1.AWSBackupConfig
	for _, config := range configs {
		if config == nil || config.ManagementCluster() != managementCluster {
			continue
		}
		if matched != nil {
			return nil, fmt.Errorf("multiple backup configurations found for management cluster %s", managementCluster)
		}
		matched = config
	}
	if matched == nil {
		return nil, fmt.Errorf("no backup configuration found for management cluster %s", managementCluster)
	}
	if matched.S3Bucket() == "" || matched.AccountId() == "" {
		return nil, fmt.Errorf("backup configuration for management cluster %s is incomplete", managementCluster)
	}
	return matched, nil
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
