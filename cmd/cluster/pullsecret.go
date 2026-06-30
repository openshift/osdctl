package cluster

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/osdctl/internal/utils/globalflags"
)

func newCmdPullSecret(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "pull-secret",
		Short:             "Diagnose and manage cluster pull secrets",
		Long:              "Diagnose and manage cluster pull secrets.",
		DisableAutoGenTag: true,
	}

	cmd.AddCommand(newCmdPullSecretAudit(streams, globalOpts))
	cmd.AddCommand(newCmdPullSecretUpdate(streams, globalOpts))
	cmd.AddCommand(newCmdPullSecretValidate())

	return cmd
}

func newCmdPullSecretValidate() *cobra.Command {
	cmd := newCmdValidatePullSecretExt()
	cmd.Use = "validate"
	cmd.Example = `  # Compare OCM Access-Token, OCM Registry-Credentials, and OCM Account Email against cluster's pull secret
  osdctl cluster pull-secret validate --cluster-id ${CLUSTER_ID} --reason "OSD-XYZ"

  # Exclude Access-Token, and Registry-Credential checks...
  osdctl cluster pull-secret validate --cluster-id ${CLUSTER_ID} --reason "OSD-XYZ" --skip-access-token --skip-registry-creds

  # Skip sending service logs (useful for testing)
  osdctl cluster pull-secret validate --cluster-id ${CLUSTER_ID} --reason "OSD-XYZ" --skip-service-logs`
	return cmd
}
