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

// newCmdAWSCredsSnapshot creates the "aws-creds snapshot" subcommand for read-only credential diagnostics.
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

AWS credentials are obtained via backplane by default, falling back to the
default AWS credential chain (env vars, ~/.aws/config). Use --aws-profile
to specify a named profile, or --aws-use-env to skip backplane and use
environment credentials directly (e.g. after rh-aws-saml-login).`,
		Example: `  # Full credential status report (uses backplane)
  osdctl account aws-creds snapshot -C $CLUSTER_ID --reason "$JIRA_TICKET"

  # Only show CredentialRequest secret status
  osdctl account aws-creds snapshot -C $CLUSTER_ID --reason "$JIRA_TICKET" --cr-secrets

  # Using rh-aws-saml-login credentials (no backplane)
  kinit $USER@IPA.REDHAT.COM
  eval $(rh-aws-saml-login --output env rhcontrol)
  export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN
  osdctl account aws-creds snapshot -C $CLUSTER_ID --reason "$JIRA_TICKET" --aws-use-env

  # With staging cluster and production hive
  osdctl account aws-creds snapshot -C $CLUSTER_ID --reason "$JIRA_TICKET" --hive-ocm-url production`,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.validate(cmd, args))
			cmdutil.CheckErr(runSnapshot(cmd.Context(), ops))
		},
	}

	ops.addFlags(cmd)
	cmd.Flags().BoolVar(&ops.crSecretsOnly, "cr-secrets", false, "Only show CredentialRequest secrets status")
	return cmd
}

// runSnapshot produces the diagnostic report, either full or CR-secrets-only based on flags.
func runSnapshot(ctx context.Context, o *awsCredsSnapshotOptions) error {

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
