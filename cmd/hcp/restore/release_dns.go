package restore

import (
	"fmt"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	hcpflags "github.com/openshift/osdctl/cmd/hcp/internal/flags"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func NewCmdReleaseDNS() *cobra.Command {
	flags := &releaseDNSFlags{}
	cmd := &cobra.Command{
		Use: "release-dns --cluster-id <cluster-id>", Short: "Release central DNS records for an HCP cluster",
		Example: "  osdctl hcp restore release-dns --cluster-id ${CLUSTER_ID}\n  osdctl hcp restore release-dns --cluster-id ${CLUSTER_ID} --profile ${AWS_PROFILE}",
		Args:    cobra.NoArgs, DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := logrus.New()
			logger.SetOutput(cmd.ErrOrStderr())
			ocmConn, err := utils.CreateConnection()
			if err != nil {
				return fmt.Errorf("creating OCM connection: %w", err)
			}
			defer ocmConn.Close()
			var releaser internalcmd.DNSReleaser = internalcmd.NewDefaultDNSReleaser(internalclient.NewOCMClient(ocmConn, internalclient.WithLogger{Logger: logger}))
			plan, err := releaser.PrepareDNSRelease(cmd.Context(), internalcmd.ReleaseDNSRequest{ClusterID: string(flags.Cluster), AWSProfile: string(flags.AWSProfile)})
			if err != nil {
				return err
			}
			if len(plan.Records) == 0 {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "No A, CNAME, or TXT records for cluster %s found in hosted zone %s. Nothing to delete.\n", plan.ClusterID, plan.ZoneName)
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "The following DNS records will be deleted for HCP cluster %s:\n", plan.ClusterID)
			fmt.Fprintf(cmd.OutOrStdout(), "  Account: %s\n  Hosted zone: %s (%s)\n", plan.AccountID, plan.ZoneName, plan.ZoneID)
			for _, record := range plan.Records {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", internalcmd.FormatDNSRecord(record))
			}
			if !confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), fmt.Sprintf("Delete these %d DNS records? (y/N): ", len(plan.Records))) {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "DNS record release cancelled.")
				return err
			}
			return releaser.ReleaseDNS(cmd.Context(), plan)
		},
	}
	flags.RegisterFlags(cmd)
	return cmd
}

type releaseDNSFlags struct {
	hcpflags.Cluster
	hcpflags.AWSProfile
}

func (f *releaseDNSFlags) RegisterFlags(cmd *cobra.Command) {
	f.AddRequired(cmd)
	f.AWSProfile.Add(cmd, "AWS profile used to assume the central DNS account role")
}
