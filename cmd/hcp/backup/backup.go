package backup

import (
	_ "embed"
	"fmt"

	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

//go:embed description.txt
var longDescription string

func NewCmdBackup() *cobra.Command {
	flags := &backupFlags{}

	cmd := &cobra.Command{
		Use:   "backup --cluster-id <cluster-id> --reason <reason>",
		Short: "Trigger a Velero backup for an HCP cluster",
		Long:  longDescription,
		Example: "  osdctl hcp backup --cluster-id 1abc2def3ghi --reason OHSS-12345\n" +
			"  osdctl hcp backup --cluster-id 1abc2def3ghi --reason OHSS-12345 --label env=prod --label incident=OHSS-12345\n" +
			"  osdctl hcp backup --cluster-id 1abc2def3ghi --reason OHSS-12345 --annotation owner=sre-team",
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

			runner := NewDefaultBackupRunner(
				ocmConn,
				WithLogger{Logger: logger},
				WithPrinter{Printer: &defaultPrinter{w: cmd.OutOrStdout()}},
			)
			return runner.Run(cmd.Context(), flags)
		},
	}

	flags.AddFlags(cmd.Flags())
	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}

// newKubeClientForCluster logs into the given cluster via backplane, reusing the
// caller's OCM connection to avoid opening a second connection. elevationReasons,
// if provided, elevates the session to backplane-cluster-admin (required for pod
// listing and exec).
func newKubeClientForCluster(ocmConn *ocmsdk.Connection, clusterID string, elevationReasons ...string) (KubeClient, error) {
	kubeCli, restCfg, clientset, err := common.GetKubeConfigAndClientWithConn(clusterID, ocmConn, elevationReasons...)
	if err != nil {
		return nil, err
	}
	return newKubeClient(kubeCli, restCfg, clientset), nil
}
