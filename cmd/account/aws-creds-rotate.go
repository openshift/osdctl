package account

import (
	"context"
	"fmt"
	"io"

	"github.com/fatih/color"
	"github.com/openshift/osdctl/pkg/controller"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type awsCredsRotateOptions struct {
	awsCredsOptions
	rotateManagedAdmin bool
	rotateCcsAdmin     bool
	refreshSecrets     bool
	dryRun             bool
}

func newCmdAWSCredsRotate(streams genericclioptions.IOStreams) *cobra.Command {
	ops := &awsCredsRotateOptions{
		awsCredsOptions: awsCredsOptions{IOStreams: streams, log: newAWSCredsLogger()},
	}

	cmd := &cobra.Command{
		Use:   "rotate -C <cluster-id> --reason <reason> [flags]",
		Short: "Rotate AWS IAM credentials for a cluster",
		Long: `Rotates AWS IAM credentials for osdManagedAdmin and/or osdCcsAdmin users.
Runs a diagnostic snapshot first, then performs the rotation with
interactive confirmation.

Use --refresh-secrets to only delete and recreate CredentialRequest secrets
without rotating AWS keys or modifying Hive secrets. This is useful when
CCO needs to re-provision secrets with existing credentials.

AWS credentials are obtained via backplane by default. Use --aws-profile
to override with a local AWS profile and manual role chaining.`,
		Example: `  # Rotate osdManagedAdmin credentials
  osdctl account aws-creds rotate -C $CLUSTER_ID --reason "$JIRA_TICKET" --managed-admin

  # Rotate osdCcsAdmin credentials (CCS clusters only)
  osdctl account aws-creds rotate -C $CLUSTER_ID --reason "$JIRA_TICKET" --ccs-admin

  # Rotate both
  osdctl account aws-creds rotate -C $CLUSTER_ID --reason "$JIRA_TICKET" --managed-admin --ccs-admin

  # Only refresh CredentialRequest secrets (no key rotation)
  osdctl account aws-creds rotate -C $CLUSTER_ID --reason "$JIRA_TICKET" --refresh-secrets

  # Dry-run: preview what would happen
  osdctl account aws-creds rotate -C $CLUSTER_ID --reason "$JIRA_TICKET" --managed-admin --dry-run

  # With staging cluster and production hive
  osdctl account aws-creds rotate -C $CLUSTER_ID --reason "$JIRA_TICKET" --managed-admin --hive-ocm-url production`,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.validateRotate(cmd, args))
			cmdutil.CheckErr(runRotate(ops))
		},
	}

	ops.addFlags(cmd)
	cmd.Flags().BoolVar(&ops.rotateManagedAdmin, "managed-admin", false, "Rotate osdManagedAdmin credentials")
	cmd.Flags().BoolVar(&ops.rotateCcsAdmin, "ccs-admin", false, "Rotate osdCcsAdmin credentials (CCS clusters only)")
	cmd.Flags().BoolVar(&ops.refreshSecrets, "refresh-secrets", false, "Only delete and recreate CredentialRequest secrets (no key rotation)")
	cmd.Flags().BoolVar(&ops.dryRun, "dry-run", false, "Preview rotation actions without making changes")

	return cmd
}

func (o *awsCredsRotateOptions) validateRotate(cmd *cobra.Command, args []string) error {
	if err := o.validate(cmd, args); err != nil {
		return err
	}
	if o.refreshSecrets && (o.rotateManagedAdmin || o.rotateCcsAdmin) {
		return cmdutil.UsageErrorf(cmd, "--refresh-secrets cannot be combined with --managed-admin or --ccs-admin")
	}
	if !o.rotateManagedAdmin && !o.rotateCcsAdmin && !o.refreshSecrets {
		return cmdutil.UsageErrorf(cmd, "at least one of --managed-admin, --ccs-admin, or --refresh-secrets is required")
	}
	return nil
}

// confirmStrictYES requires the user to type "YES" exactly to proceed.
func confirmStrictYES(in io.Reader, out io.Writer) bool {
	fmt.Fprintf(out, "Type YES to continue: ")
	var response string
	if _, err := fmt.Fscanln(in, &response); err != nil {
		return false
	}
	return response == "YES"
}

func runRotate(o *awsCredsRotateOptions) error {
	ctx := context.TODO()

	rc, err := o.identifyCluster()
	if err != nil {
		return err
	}
	defer rc.ocmConn.Close()

	if o.rotateCcsAdmin && !rc.isCCS {
		return fmt.Errorf("--ccs-admin specified but cluster is not CCS/BYOC")
	}

	if o.refreshSecrets {
		if err := o.resolveForCRSecrets(ctx, rc); err != nil {
			return err
		}
		report, err := controller.DiagnoseCRSecrets(ctx, rc.hiveClient, rc.managedClient, rc.claimName, rc.account, o.Out)
		if err != nil {
			return err
		}
		controller.RenderCredRequestTable(report, o.Out)
		return runRefreshSecrets(o, rc, report)
	}

	if err := o.resolveCluster(ctx, rc); err != nil {
		return err
	}

	o.log.Info("Running pre-rotation diagnostic snapshot")
	input := rc.toCredsInput(o.log, o.Out)
	report, err := controller.DiagnoseCredentials(ctx, input)
	if err != nil {
		return err
	}
	controller.RenderReport(report, o.Out)

	if !report.AllPermissionsOK {
		o.log.Warn("IAM permission check detected issues")
		red := color.New(color.FgRed).SprintFunc()
		fmt.Fprintf(o.Out, "\n%s Pre-flight permission checks detected issues.\n", red("[WARN]"))
		fmt.Fprintln(o.Out, "Proceeding may result in failures during rotation or CR secret recreation.")
		if !confirmStrictYES(o.In, o.Out) {
			o.log.Info("Operation cancelled by user")
			return nil
		}
	}

	if o.dryRun {
		o.log.Info("Dry-run mode — no changes will be made")
	}

	rotateInput := &controller.RotateSecretInput{
		AccountCRName:           rc.claimName,
		Account:                 rc.account,
		OsdManagedAdminUsername: rc.adminUsername,
		UpdateManagedAdminCreds: o.rotateManagedAdmin,
		UpdateCcsCreds:          o.rotateCcsAdmin,
		DryRun:                  o.dryRun,
		AwsClient:               rc.awsClient,
		HiveKubeClient:          rc.hiveClient,
		ManagedClusterClient:    rc.managedClient,
		Report:                  report,
		Log:                     o.log,
		Out:                     o.Out,
	}

	if !o.dryRun {
		o.log.Warn("Credential rotation will modify IAM keys and Hive secrets")
		fmt.Fprintln(o.Out, "\nProceed with credential rotation?")
		if !utils.ConfirmPrompt() {
			o.log.Info("Rotation cancelled by user")
			return nil
		}
	}

	o.log.Info("Starting credential rotation")
	if err := controller.RotateSecret(ctx, rotateInput); err != nil {
		o.log.WithError(err).Error("Credential rotation failed")
		return err
	}
	o.log.Info("Credential rotation completed successfully")
	return nil
}

func runRefreshSecrets(o *awsCredsRotateOptions, rc *resolvedCluster, report *controller.DiagnosticReport) error {
	ctx := context.TODO()

	if rc.managedClient == nil {
		return fmt.Errorf("managed cluster client not available — cannot refresh secrets")
	}

	if o.dryRun {
		o.log.Info("Dry-run mode — no secrets will be deleted")
		fmt.Fprintln(o.Out, "\n[Dry Run] Would delete and recreate all CredentialRequest secrets.")
		fmt.Fprintln(o.Out, "[Dry Run] No AWS keys or Hive secrets would be modified.")
		return nil
	}

	fmt.Fprintf(o.Out, "\nThis will delete %d CredentialRequest secret(s) so CCO recreates them.\n", len(report.CredRequests))
	fmt.Fprintln(o.Out, "No AWS keys or Hive secrets will be modified.")
	fmt.Fprintln(o.Out, "\nProceed with secret refresh?")
	if !utils.ConfirmPrompt() {
		o.log.Info("Refresh cancelled by user")
		return nil
	}

	o.log.Info("Deleting credential secrets for CCO to recreate")
	if err := controller.DeleteCredentialSecrets(ctx, rc.managedClient, o.Out); err != nil {
		o.log.WithError(err).Error("Failed to refresh credential secrets")
		return err
	}

	o.log.Info("Credential secret refresh completed successfully")
	return nil
}
