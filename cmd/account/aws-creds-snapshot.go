package account

import (
	"context"

	"github.com/openshift/osdctl/pkg/controller"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type awsCredsSnapshotOptions struct {
	awsCredsOptions
	crSecretsOnly bool
}

func newCmdAWSCredsSnapshot(streams genericclioptions.IOStreams) *cobra.Command {
	ops := &awsCredsSnapshotOptions{
		awsCredsOptions: awsCredsOptions{IOStreams: streams, log: newAWSCredsLogger()},
	}

	cmd := &cobra.Command{
		Use:   "snapshot -C <cluster-id> --reason <reason> [flags]",
		Short: "Show a read-only credential status report for a cluster",
		Long: `Produces a diagnostic report of AWS IAM credentials including:
  - IAM access keys and which Hive secrets reference them
  - CredentialRequest secrets and whether they need refresh
  - IAM permission simulation (SCP/policy restriction detection)

Use --cr-secrets to show only the CredentialRequest secrets table.

This is a read-only operation — no credentials are modified.

AWS credentials are obtained via backplane by default. Use --aws-profile
to override with a local AWS profile and manual role chaining.`,
		Example: `  # Full credential status report
  osdctl account aws-creds snapshot -C $CLUSTER_ID --reason "$JIRA_TICKET"

  # Only show CredentialRequest secret status
  osdctl account aws-creds snapshot -C $CLUSTER_ID --reason "$JIRA_TICKET" --cr-secrets

  # With staging cluster and production hive
  osdctl account aws-creds snapshot -C $CLUSTER_ID --reason "$JIRA_TICKET" --hive-ocm-url production`,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.validate(cmd, args))
			cmdutil.CheckErr(runSnapshot(ops))
		},
	}

	ops.addFlags(cmd)
	cmd.Flags().BoolVar(&ops.crSecretsOnly, "cr-secrets", false, "Only show CredentialRequest secrets status")
	return cmd
}

func runSnapshot(o *awsCredsSnapshotOptions) error {
	ctx := context.TODO()

	rc, err := o.identifyCluster()
	if err != nil {
		return err
	}
	defer rc.ocmConn.Close()

	if o.crSecretsOnly {
		if err := o.resolveForCRSecrets(ctx, rc); err != nil {
			return err
		}
		report, err := controller.DiagnoseCRSecrets(ctx, rc.hiveClient, rc.managedClient, rc.claimName, rc.account, o.Out)
		if err != nil {
			return err
		}
		controller.RenderCredRequestTable(report, o.Out)
		return nil
	}

	if err := o.resolveCluster(ctx, rc); err != nil {
		return err
	}

	input := rc.toCredsInput(o.log, o.Out)
	report, err := controller.DiagnoseCredentials(ctx, input)
	if err != nil {
		return err
	}
	controller.RenderReport(report, o.Out)
	return nil
}
