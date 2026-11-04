package backup

import (
	"fmt"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	hcpflags "github.com/openshift/osdctl/cmd/hcp/internal/flags"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewCmdDiscover() *cobra.Command {
	flags := &discoveryFlags{
		Limit: 1,
	}
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
				ClusterID: string(flags.Cluster), Limit: flags.Limit, AWSProfile: string(flags.AWSProfile),
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
	flags.RegisterFlags(cmd)
	return cmd
}

type discoveryFlags struct {
	hcpflags.Cluster
	hcpflags.AWSProfile
	Limit int
}

func (f *discoveryFlags) RegisterFlags(cmd *cobra.Command) {
	f.AddRequired(cmd)
	f.AWSProfile.Add(cmd, "AWS profile used to assume the DR account role")
	cmd.Flags().IntVarP(&f.Limit, "limit", "l", f.Limit, "Number of most recent backups to display (-1 displays all)")
}
