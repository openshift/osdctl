package backup

import (
	"fmt"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type discoveryFlags struct {
	clusterID  string
	limit      int
	awsProfile string
}

func NewCmdDiscover() *cobra.Command {
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
			var discoverer internalcmd.BackupDiscoverer = internalcmd.NewDefaultBackupDiscoverer(internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: logger}))
			backups, err := discoverer.DiscoverBackups(cmd.Context(), internalcmd.DiscoveryRequest{
				ClusterID: flags.clusterID, Limit: flags.limit, AWSProfile: flags.awsProfile,
			})
			if err != nil {
				return err
			}
			for _, backup := range backups {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), backup.ID); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&flags.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().IntVarP(&flags.limit, "limit", "l", 1, "Number of most recent backups to display (-1 displays all)")
	cmd.Flags().StringVarP(&flags.awsProfile, "profile", "p", "", "AWS profile used to assume the DR account role")
	_ = cmd.MarkFlagRequired("cluster-id")
	return cmd
}
