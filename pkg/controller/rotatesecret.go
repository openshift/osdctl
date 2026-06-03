package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	ccov1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	hiveapiv1 "github.com/openshift/hive/apis/hive/v1"
	hiveinternalv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/sirupsen/logrus"
	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	awsAccountNamespace = "aws-account-operator"
	osdManagedAdminIAM  = "osdManagedAdmin"
)

// RotationRequiredActions are the IAM actions needed by the rotation tooling
// to create/delete access keys and manage IAM users.
var RotationRequiredActions = []string{
	"iam:CreateAccessKey",
	"iam:CreateUser",
	"iam:DeleteAccessKey",
	"iam:DeleteUser",
	"iam:DeleteUserPolicy",
	"iam:GetUser",
	"iam:GetUserPolicy",
	"iam:ListAccessKeys",
	"iam:PutUserPolicy",
	"iam:TagUser",
}

// confirmFrom prompts for y/N confirmation reading from the given reader.
func confirmFrom(in io.Reader) bool {
	var response string
	if _, err := fmt.Fscanln(in, &response); err != nil {
		return false
	}
	switch strings.ToLower(response) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// InsufficientPermissionsError is returned when SimulatePrincipalPolicy
// reports that one or more required IAM actions are denied.
type InsufficientPermissionsError struct {
	DeniedActions []string
}

// Error returns a human-readable message listing the denied IAM actions.
func (e *InsufficientPermissionsError) Error() string {
	return fmt.Sprintf("insufficient permissions for secret rotation. Denied actions: %v", e.DeniedActions)
}

// SyncPollInterval is the delay between ClusterSync status checks.
var SyncPollInterval = 5 * time.Second

// SyncMaxRetries is the maximum number of ClusterSync poll attempts.
var SyncMaxRetries = 6

// RotateSecretInput holds all resolved dependencies for secret rotation.
// The CLI layer is responsible for resolving AWS and k8s clients before
// calling RotateSecret.
type RotateSecretInput struct {
	// AccountCRName is the name of the Account CR.
	AccountCRName string

	// Account is the pre-fetched Account CR.
	Account *awsv1alpha1.Account

	// OsdManagedAdminUsername is an explicit admin username override.
	// If empty, it is derived from the Account CR's iamUserId label.
	OsdManagedAdminUsername string

	// UpdateManagedAdminCreds controls whether osdManagedAdmin credentials are rotated.
	UpdateManagedAdminCreds bool

	// UpdateCcsCreds controls whether osdCcsAdmin credentials are also rotated.
	UpdateCcsCreds bool

	// AwsClient is the fully-authenticated AWS client with permissions in the
	// target AWS account (after all role chaining has been completed).
	AwsClient awsprovider.Client

	// HiveKubeClient is the k8s client connected to the hive cluster.
	HiveKubeClient client.Client

	// ManagedClusterClient is the k8s client connected to the managed cluster
	// (via backplane using the target OCM environment). Used to delete
	// CredentialRequests so CCO recreates them with the new credentials.
	ManagedClusterClient client.Client

	// DryRun, when true, prints what actions would be taken without performing
	// any mutating operations (no AWS key creation/deletion, no k8s resource
	// creation/deletion/updates).
	DryRun bool

	// Report is the pre-computed diagnostic report from the snapshot phase.
	// Used to display key/secret context during interactive key deletion.
	Report *DiagnosticReport

	// Log is the logger for operational messages (writes to stderr).
	Log *logrus.Logger

	// SkipPermissionCheck skips the managed-admin permission verification when
	// the caller has already run diagnostics and the user confirmed despite failures.
	SkipPermissionCheck bool

	// In is the reader for interactive prompts (defaults to os.Stdin).
	In io.Reader

	// Out is the writer for structured report output (writes to stdout).
	Out io.Writer
}

// RotateSecret performs the IAM credential rotation workflow:
//  1. Validates the Account CR (not STS, has iamUserId label)
//  2. Resolves the osdManagedAdmin username
//  3. Verifies rotation permissions via SimulatePrincipalPolicy
//  4. Creates a new IAM access key
//  5. Updates k8s secrets on hive
//  6. Creates a SyncSet to push credentials to the cluster
//  7. Polls ClusterSync for completion and cleans up the SyncSet
//  8. Optionally rotates osdCcsAdmin credentials
func RotateSecret(ctx context.Context, input *RotateSecretInput) error {
	log := getLog(input.Log)
	account := input.Account

	if account.Spec.ManualSTSMode {
		return fmt.Errorf("account %s is STS - No IAM User Credentials to Rotate", input.AccountCRName)
	}

	accountID := account.Spec.AwsAccountID

	// Managed-admin username resolution — only needed when rotating managed-admin creds
	var adminUsername string
	if input.UpdateManagedAdminCreds {
		accountIDSuffixLabel, ok := account.Labels["iamUserId"]
		if !ok {
			return fmt.Errorf("no label on Account CR for IAM User")
		}

		adminUsername = input.OsdManagedAdminUsername
		if adminUsername == "" {
			adminUsername = osdManagedAdminIAM + "-" + accountIDSuffixLabel
		}

		if !input.SkipPermissionCheck {
			var err error
			adminUsername, err = resolveAdminUsername(input.Out, input.AwsClient, accountID, adminUsername, accountIDSuffixLabel)
			if err != nil {
				return err
			}
		}
	}

	if input.DryRun {
		dr := &dryRunChecker{out: input.Out, allOK: true}
		ns := account.Spec.ClaimLinkNamespace
		secretName := input.AccountCRName + "-secret"

		if input.UpdateManagedAdminCreds {
			// AWS: verify IAM access
			dr.would("create a new IAM access key for user: %s", adminUsername)
			_, listErr := input.AwsClient.ListAccessKeys(&iam.ListAccessKeysInput{UserName: awsSdk.String(adminUsername)})
			dr.report(listErr == nil, "AWS: ListAccessKeys for %s", adminUsername)
			dr.would("list access keys and report old keys to remove")

			// Hive: verify secret access + RBAC for update
			dr.would("update secret %s/%s with new credentials", awsAccountNamespace, secretName)
			dr.checkResourceAccess(ctx, input.HiveKubeClient, "Hive", "update", "secrets", awsAccountNamespace, secretName)

			dr.would("update secret %s/%s with new credentials", ns, "aws")
			dr.checkResourceAccess(ctx, input.HiveKubeClient, "Hive", "update", "secrets", ns, "aws")

			// Hive: verify ClusterDeployment exists + SyncSet RBAC
			dr.would("create SyncSet %s/%s to sync credentials to cluster", ns, "aws-sync")
			dr.info("SyncSet is created fresh during rotation (not expected to exist beforehand)")
			cdList := &hiveapiv1.ClusterDeploymentList{}
			listCDErr := input.HiveKubeClient.List(ctx, cdList, client.InNamespace(ns))
			dr.report(listCDErr == nil && len(cdList.Items) > 0, "Hive: LIST ClusterDeployments in %s", ns)
			dr.checkCanI(ctx, input.HiveKubeClient, "Hive", "create", "syncsets", "hive.openshift.io", ns)
			dr.checkCanI(ctx, input.HiveKubeClient, "Hive", "delete", "syncsets", "hive.openshift.io", ns)

			dr.would("poll ClusterSync and then delete SyncSet %s/%s", ns, "aws-sync")

			// Managed cluster: verify credential secret access
			if input.ManagedClusterClient == nil {
				dr.info("Managed cluster client not available — credential secret checks skipped")
			} else if err := dryRunDeleteCredentialSecrets(ctx, input.ManagedClusterClient, input.Out, dr); err != nil {
				return err
			}
		}

		if input.UpdateCcsCreds {
			if account.Spec.BYOC {
				dr.would("create a new IAM access key for user: osdCcsAdmin")
				_, ccsListErr := input.AwsClient.ListAccessKeys(&iam.ListAccessKeysInput{UserName: awsSdk.String("osdCcsAdmin")})
				dr.report(ccsListErr == nil, "AWS: ListAccessKeys for osdCcsAdmin")

				dr.would("update secret %s/%s with new osdCcsAdmin credentials", ns, "byoc")
				dr.checkResourceAccess(ctx, input.HiveKubeClient, "Hive", "update", "secrets", ns, "byoc")
			} else {
				fmt.Fprintf(input.Out, "%s Account is not CCS, would skip osdCcsAdmin credential rotation\n", colorBlue("[Dry Run]"))
			}
		}

		if dr.allOK {
			fmt.Fprintf(input.Out, "\n%s %s All pre-flight checks passed. No changes were made.\n", colorBlue("[Dry Run]"), colorGreen("OK"))
		} else {
			fmt.Fprintf(input.Out, "\n%s %s Some pre-flight checks failed. Resolve issues before rotating.\n", colorBlue("[Dry Run]"), colorRed("FAIL"))
		}
		return nil
	}

	if err := preflightCheckArtifacts(ctx, input); err != nil {
		return err
	}

	var completedSteps []string
	reportFailure := func(failedStep string, err error) error {
		log.WithError(err).Error(failedStep)
		fmt.Fprintf(input.Out, "\n%s Rotation failed at: %s\n", colorRed("[ERROR]"), failedStep)
		if len(completedSteps) > 0 {
			fmt.Fprintln(input.Out, "\nCompleted steps before failure:")
			for _, step := range completedSteps {
				fmt.Fprintf(input.Out, "  %s %s\n", colorGreen("[DONE]"), step)
			}
		}
		fmt.Fprintf(input.Out, "  %s %s: %v\n", colorRed("[FAIL]"), failedStep, err)
		fmt.Fprintln(input.Out, "\nThis command can be re-run safely. Already-completed steps will be retried.")
		return err
	}

	if input.UpdateManagedAdminCreds {
		if input.UpdateCcsCreds {
			fmt.Fprintf(input.Out, "\n%s\n", "==========================================================================")
			fmt.Fprintf(input.Out, " Phase 1: Rotating %s credentials\n", adminUsername)
			fmt.Fprintf(input.Out, "%s\n\n", "==========================================================================")
		}

		createAccessKeyOutput, err := createAccessKeyWithRetry(input.AwsClient, adminUsername, accountID, input.Report, input.In, input.Out)
		if err != nil {
			return reportFailure("Create new IAM access key", err)
		}
		if createAccessKeyOutput == nil || createAccessKeyOutput.AccessKey == nil {
			return reportFailure("Create new IAM access key", fmt.Errorf("AWS returned nil access key"))
		}
		adminUsername = *createAccessKeyOutput.AccessKey.UserName
		newKeyID := *createAccessKeyOutput.AccessKey.AccessKeyId
		completedSteps = append(completedSteps, fmt.Sprintf("Created new access key %s for %s", truncateKeyID(newKeyID), adminUsername))

		rotationCommitted := false
		defer func() {
			if rotationCommitted {
				return
			}
			log.Warn("Rotation did not complete — cleaning up newly created access key")
			if _, delErr := input.AwsClient.DeleteAccessKey(&iam.DeleteAccessKeyInput{
				UserName:    awsSdk.String(adminUsername),
				AccessKeyId: awsSdk.String(newKeyID),
			}); delErr != nil {
				log.WithError(delErr).Errorf("Failed to delete orphaned access key %s for %s — manual cleanup required", truncateKeyID(newKeyID), adminUsername)
				fmt.Fprintf(input.Out, "\n%s Failed to clean up access key %s for %s: %v\n", colorRed("[ERROR]"), truncateKeyID(newKeyID), adminUsername, delErr)
				fmt.Fprintln(input.Out, "  Manual cleanup required: aws iam delete-access-key --user-name", adminUsername, "--access-key-id", newKeyID)
			}
		}()

		if err := reportAccessKeys(input.AwsClient, adminUsername, newKeyID, input.Out); err != nil {
			return reportFailure("List access keys", err)
		}

		newSecretData := map[string][]byte{
			"aws_user_name":         []byte(*createAccessKeyOutput.AccessKey.UserName),
			"aws_access_key_id":     []byte(newKeyID),
			"aws_secret_access_key": []byte(*createAccessKeyOutput.AccessKey.SecretAccessKey),
		}

		log.Info("Updating AAO account secret on Hive")
		if err := updateSecret(ctx, input.HiveKubeClient, input.AccountCRName+"-secret", awsAccountNamespace, newSecretData); err != nil {
			return reportFailure("Update AAO account secret on Hive", err)
		}
		completedSteps = append(completedSteps, fmt.Sprintf("Updated secret %s/%s", awsAccountNamespace, input.AccountCRName+"-secret"))

		rotationCommitted = true

		log.Info("Updating cluster namespace secret on Hive")
		if err := updateSecret(ctx, input.HiveKubeClient, "aws", account.Spec.ClaimLinkNamespace, newSecretData); err != nil {
			return reportFailure("Update cluster namespace secret on Hive", err)
		}
		completedSteps = append(completedSteps, fmt.Sprintf("Updated secret %s/aws", account.Spec.ClaimLinkNamespace))

		fmt.Fprintln(input.Out, "AWS creds updated on hive.")

		log.Info("Syncing credentials to cluster via SyncSet")
		if err := syncCredentialsToCluster(ctx, input.HiveKubeClient, account.Spec.ClaimLinkNamespace, input.Out); err != nil {
			return reportFailure("Sync credentials to cluster via SyncSet", err)
		}
		completedSteps = append(completedSteps, "Synced credentials to cluster via SyncSet")

		log.WithField("user", adminUsername).Info("Successfully rotated access keys")
		fmt.Fprintf(input.Out, "Successfully rotated access keys for %s\n", adminUsername)

		if input.ManagedClusterClient == nil {
			log.Warn("Managed cluster client not available — skipping credential secret deletion")
			fmt.Fprintln(input.Out, "\nManaged cluster client not available. Delete credential secrets manually:")
			fmt.Fprintln(input.Out, "  oc get credentialsrequests -A -o wide")
			completedSteps = append(completedSteps, "Skipped credential secret deletion (managed cluster not connected)")
		} else {
			log.Info("Deleting credential secrets so CCO recreates them")
			if err := DeleteCredentialSecrets(ctx, input.ManagedClusterClient, input.In, input.Out); err != nil {
				return reportFailure("Delete credential secrets on managed cluster", err)
			}
			completedSteps = append(completedSteps, "Deleted credential secrets for CCO to recreate")
		}
	}

	if input.UpdateCcsCreds {
		fmt.Fprintf(input.Out, "\n%s\n", "==========================================================================")
		if input.UpdateManagedAdminCreds {
			fmt.Fprintf(input.Out, " Phase 2: Rotating osdCcsAdmin credentials\n")
		} else {
			fmt.Fprintf(input.Out, " Rotating osdCcsAdmin credentials\n")
		}
		fmt.Fprintf(input.Out, "%s\n\n", "==========================================================================")
		log.Info("Rotating osdCcsAdmin credentials")
		if err := rotateCcsAdminCredentials(ctx, input.AwsClient, input.HiveKubeClient, account, input.Report, input.Log, input.In, input.Out); err != nil {
			return reportFailure("Rotate osdCcsAdmin credentials", err)
		}
		completedSteps = append(completedSteps, "Rotated osdCcsAdmin credentials")
	}

	fmt.Fprintln(input.Out, "\nPost-rotation: verifying cluster health...")
	if err := postRotationCheck(ctx, input, adminUsername); err != nil {
		fmt.Fprintf(input.Out, "%s Post-rotation check encountered issues: %v\n", colorYellow("[WARN]"), err)
		fmt.Fprintln(input.Out, "Rotation completed but manual verification recommended.")
	}

	return nil
}

// VerifyRotationPermissions checks if the assumed role has the necessary IAM
// permissions to perform secret rotation by simulating the required actions.
// Uses simulateActions internally for consistency with the diagnostic path.
func VerifyRotationPermissions(out io.Writer, awsClient awsprovider.Client, accountID string, username string) error {
	fmt.Fprintf(out, "Verifying IAM permissions for user %s...\n", username)

	report := &DiagnosticReport{AllPermissionsOK: true}
	userArn := fmt.Sprintf("arn:aws:iam::%s:user/%s", accountID, username)

	if err := simulateActions(awsClient, userArn, RotationRequiredActions, "rotation", nil, report); err != nil {
		return fmt.Errorf("failed to simulate principal policy: %w", err)
	}

	if !report.AllPermissionsOK && len(report.Permissions) == 0 {
		return fmt.Errorf("failed to simulate principal policy: permission check could not be completed")
	}

	var deniedActions []string
	for _, p := range report.Permissions {
		if !p.Allowed {
			deniedActions = append(deniedActions, p.Action)
		}
	}

	if len(deniedActions) > 0 {
		return &InsufficientPermissionsError{DeniedActions: deniedActions}
	}

	fmt.Fprintln(out, "Permission verification successful. All required IAM actions are allowed.")
	return nil
}

// createAccessKeyWithRetry attempts to create an IAM access key, falling back to
// the unsuffixed admin user on NoSuchEntity and prompting for interactive key
// deletion when the 2-key limit is reached.
func createAccessKeyWithRetry(awsClient awsprovider.Client, username, accountID string, report *DiagnosticReport, in io.Reader, out io.Writer) (*iam.CreateAccessKeyOutput, error) {
	output, err := awsClient.CreateAccessKey(&iam.CreateAccessKeyInput{
		UserName: awsSdk.String(username),
	})
	if err == nil {
		return output, nil
	}

	var nse *iamTypes.NoSuchEntityException
	if errors.As(err, &nse) {
		// Only fall back to unsuffixed osdManagedAdmin for suffixed managed-admin users
		if strings.HasPrefix(username, osdManagedAdminIAM) && username != osdManagedAdminIAM {
			if err := VerifyRotationPermissions(out, awsClient, accountID, osdManagedAdminIAM); err != nil {
				return nil, err
			}
			return awsClient.CreateAccessKey(&iam.CreateAccessKeyInput{
				UserName: awsSdk.String(osdManagedAdminIAM),
			})
		}
		return nil, fmt.Errorf("IAM user %s not found: %w", username, nse)
	}

	if !isLimitExceeded(err) {
		return nil, err
	}

	fmt.Fprintf(out, "\n%s IAM user %s already has the maximum number of access keys (2).\n", colorYellow("[WARN]"), username)
	fmt.Fprintln(out, "A key must be deleted before a new one can be created.")
	fmt.Fprintln(out, "Review the keys below and select which one to delete:")

	userKeys := filterKeysForUser(report, username)
	if len(userKeys) == 0 {
		return nil, fmt.Errorf("no key data available for %s — run snapshot first", username)
	}

	renderKeySelectionTable(userKeys, report, out)

	fmt.Fprintf(out, "\nEnter the number of the key to delete (or 'q' to quit): ")
	var choice string
	if _, scanErr := fmt.Fscanln(in, &choice); scanErr != nil || choice == "q" || choice == "Q" {
		return nil, fmt.Errorf("key deletion cancelled by user")
	}

	idx := 0
	if _, parseErr := fmt.Sscanf(choice, "%d", &idx); parseErr != nil || idx < 1 || idx > len(userKeys) {
		return nil, fmt.Errorf("invalid selection: %s", choice)
	}

	keyToDelete := userKeys[idx-1]
	fmt.Fprintf(out, "Deleting access key %s...\n", truncateKeyID(keyToDelete.AccessKeyID))
	if _, delErr := awsClient.DeleteAccessKey(&iam.DeleteAccessKeyInput{
		UserName:    awsSdk.String(username),
		AccessKeyId: awsSdk.String(keyToDelete.AccessKeyID),
	}); delErr != nil {
		return nil, fmt.Errorf("failed to delete access key %s: %w", keyToDelete.AccessKeyID, delErr)
	}
	fmt.Fprintf(out, "%s Deleted access key %s\n", colorGreen("[OK]"), truncateKeyID(keyToDelete.AccessKeyID))

	fmt.Fprintln(out, "Creating new access key...")
	return awsClient.CreateAccessKey(&iam.CreateAccessKeyInput{
		UserName: awsSdk.String(username),
	})
}

// filterKeysForUser returns the subset of diagnostic report keys belonging to the given IAM user.
func filterKeysForUser(report *DiagnosticReport, username string) []KeyStatus {
	if report == nil {
		return nil
	}
	var keys []KeyStatus
	for _, k := range report.Keys {
		if k.UserName == username {
			keys = append(keys, k)
		}
	}
	return keys
}

// renderKeySelectionTable prints an interactive numbered table of IAM keys with
// Hive secret references, highlighting the recommended deletion candidate.
func renderKeySelectionTable(keys []KeyStatus, report *DiagnosticReport, out io.Writer) {
	secretsByKeyID := map[string][]string{}
	if report != nil {
		for _, s := range report.Secrets {
			if s.Exists && s.AccessKeyID != "" {
				secretsByKeyID[s.AccessKeyID] = append(secretsByKeyID[s.AccessKeyID], s.SecretName)
			}
		}
	}

	if len(keys) > 0 {
		fmt.Fprintf(out, "\n  IAM User: %s\n", keys[0].UserName)
	}
	fmt.Fprintf(out, "  %-4s %-24s %-6s %-10s %-8s %-34s %s\n", "#", "KEY ID", "AGE", "LAST USED", "STATUS", "REF'd by HIVE SECRET(S)", "SYNC")
	fmt.Fprintf(out, "  %-4s %-24s %-6s %-10s %-8s %-34s %s\n", underline(4), underline(24), underline(6), underline(10), underline(8), underline(34), underline(6))

	suggestedIdx := -1
	for i, k := range keys {
		secretNames := secretsByKeyID[k.AccessKeyID]
		secretStr := colorBlue("(not referenced by this cluster)")
		syncStr := "--"
		if len(secretNames) > 0 {
			secretStr = strings.Join(secretNames, ", ")
			syncStr = colorGreen("OK")
		} else {
			if suggestedIdx == -1 {
				suggestedIdx = i
			} else if k.Age > keys[suggestedIdx].Age {
				suggestedIdx = i
			}
		}

		visibleSecretStr := secretStr
		if len(secretNames) > 0 {
			visibleSecretStr = truncate(secretStr, 38)
		}

		marker := fmt.Sprintf("%d", i+1)
		if i == suggestedIdx {
			marker = colorYellow(fmt.Sprintf("%d *", i+1))
		}

		fmt.Fprintf(out, "  %-4s %-24s %-6s %-10s %-8s %-34s %s\n",
			marker,
			k.AccessKeyID,
			formatAge(k.Age),
			k.LastUsed,
			k.Status,
			visibleSecretStr,
			syncStr,
		)
	}

	if suggestedIdx >= 0 {
		fmt.Fprintf(out, "\n  %s Key #%d is not referenced by this cluster and is the recommended candidate for deletion.\n",
			colorYellow("*"), suggestedIdx+1)
	}
}

// isLimitExceeded returns true if the error is an IAM LimitExceededException (e.g., max 2 access keys).
func isLimitExceeded(err error) bool {
	if err == nil {
		return false
	}
	var le *iamTypes.LimitExceededException
	return errors.As(err, &le)
}

// resolveAdminUsername verifies rotation permissions for the given username,
// falling back to the unsuffixed osdManagedAdmin only when the suffixed user
// has insufficient permissions. Transport or API errors are never retried
// with a different principal.
func resolveAdminUsername(out io.Writer, awsClient awsprovider.Client, accountID, username, suffix string) (string, error) {
	err := VerifyRotationPermissions(out, awsClient, accountID, username)
	if err != nil {
		var permErr *InsufficientPermissionsError
		if errors.As(err, &permErr) && username == osdManagedAdminIAM+"-"+suffix {
			fmt.Fprintf(out, "Permission verification failed for %s, trying %s...\n", username, osdManagedAdminIAM)
			if err := VerifyRotationPermissions(out, awsClient, accountID, osdManagedAdminIAM); err != nil {
				return "", err
			}
			return osdManagedAdminIAM, nil
		}
		return "", err
	}
	return username, nil
}

// reportAccessKeys lists all access keys for a user and prints each one,
// highlighting which is the newly created key and which are old keys that
// should be removed manually.
func reportAccessKeys(awsClient awsprovider.Client, username, newKeyID string, out io.Writer) error {
	listOutput, err := awsClient.ListAccessKeys(&iam.ListAccessKeysInput{
		UserName: awsSdk.String(username),
	})
	if err != nil {
		return fmt.Errorf("failed to list access keys for user %s: %w", username, err)
	}

	fmt.Fprintf(out, "\nAccess keys for IAM user %s:\n", username)
	for _, key := range listOutput.AccessKeyMetadata {
		if key.AccessKeyId == nil {
			continue
		}
		if *key.AccessKeyId == newKeyID {
			fmt.Fprintf(out, "  - %s (new - just created)\n", truncateKeyID(*key.AccessKeyId))
		} else {
			fmt.Fprintf(out, "  - %s (old - should be removed)\n", truncateKeyID(*key.AccessKeyId))
		}
	}

	hasOldKeys := false
	for _, key := range listOutput.AccessKeyMetadata {
		if key.AccessKeyId == nil {
			continue
		}
		if *key.AccessKeyId != newKeyID {
			hasOldKeys = true
			break
		}
	}
	if hasOldKeys {
		fmt.Fprintf(out, "\nThe old access key(s) listed above should be removed once rotation is finished.\n")
		fmt.Fprintf(out, "\n")
	}

	return nil
}

// postRotationCheck validates that cluster artifacts are healthy after rotation.
func postRotationCheck(ctx context.Context, input *RotateSecretInput, adminUsername string) error {
	account := input.Account
	ns := account.Spec.ClaimLinkNamespace
	allOK := true

	postReport := &DiagnosticReport{
		IsCCS:         account.Spec.BYOC,
		AWSAccountID:  account.Spec.AwsAccountID,
		AccountCRName: input.AccountCRName,
	}
	hiveKeyIDs := map[string]string{}

	type postSecretCheck struct {
		name      string
		namespace string
	}
	var secretChecks []postSecretCheck
	if input.UpdateManagedAdminCreds {
		secretChecks = append(secretChecks,
			postSecretCheck{input.AccountCRName + "-secret", awsAccountNamespace},
			postSecretCheck{"aws", ns},
		)
	}
	if input.UpdateCcsCreds && account.Spec.BYOC {
		secretChecks = append(secretChecks,
			postSecretCheck{"byoc", ns},
		)
	}

	for _, sc := range secretChecks {
		ss := SecretStatus{SecretName: sc.name, Namespace: sc.namespace}
		secret := &corev1.Secret{}
		if err := input.HiveKubeClient.Get(ctx, types.NamespacedName{Name: sc.name, Namespace: sc.namespace}, secret); err != nil {
			ss.Exists = false
			allOK = false
		} else {
			ss.Exists = true
			if keyID, ok := secret.Data["aws_access_key_id"]; ok {
				ss.AccessKeyID = string(keyID)
				hiveKeyIDs[sc.name] = string(keyID)
			}
		}
		postReport.Secrets = append(postReport.Secrets, ss)
	}

	var users []string
	if adminUsername != "" {
		users = append(users, adminUsername)
	}
	if input.UpdateCcsCreds && account.Spec.BYOC {
		users = append(users, "osdCcsAdmin")
	}

	newestKeyByUser := map[string]string{}
	for _, username := range users {
		listOutput, err := input.AwsClient.ListAccessKeys(&iam.ListAccessKeysInput{UserName: awsSdk.String(username)})
		if err != nil {
			fmt.Fprintf(input.Out, "  %s Could not list access keys for %s: %v\n", colorRed("[FAIL]"), username, err)
			allOK = false
			continue
		}

		var matchKeyID string
		if username == "osdCcsAdmin" {
			matchKeyID = hiveKeyIDs["byoc"]
		} else {
			matchKeyID = hiveKeyIDs[input.AccountCRName+"-secret"]
		}

		var newestDate time.Time
		for _, key := range listOutput.AccessKeyMetadata {
			if key.AccessKeyId == nil {
				continue
			}
			ks := KeyStatus{
				UserName:    username,
				AccessKeyID: *key.AccessKeyId,
				Status:      string(key.Status),
			}
			if key.CreateDate != nil {
				ks.CreateDate = *key.CreateDate
				ks.Age = time.Since(*key.CreateDate)
				if key.CreateDate.After(newestDate) {
					newestDate = *key.CreateDate
					newestKeyByUser[username] = *key.AccessKeyId
				}
			}
			if matchKeyID != "" && *key.AccessKeyId == matchKeyID {
				ks.HiveMatch = true
			}
			postReport.Keys = append(postReport.Keys, ks)
		}
	}

	for i := range postReport.Secrets {
		ss := &postReport.Secrets[i]
		if !ss.Exists || ss.AccessKeyID == "" {
			continue
		}
		for _, ks := range postReport.Keys {
			if ks.AccessKeyID == ss.AccessKeyID {
				ss.MatchesAWS = true
				break
			}
		}
	}

	renderCredentialsTable(postReport, input.Out)
	fmt.Fprintln(input.Out)

	for _, ss := range postReport.Secrets {
		expectedKey := newestKeyByUser[adminUsername]
		if ss.SecretName == "byoc" {
			expectedKey = newestKeyByUser["osdCcsAdmin"]
		}
		if !ss.Exists {
			fmt.Fprintf(input.Out, "  %s Hive secret %s/%s not found\n", colorRed("[FAIL]"), ss.Namespace, ss.SecretName)
			allOK = false
		} else if expectedKey != "" && ss.AccessKeyID == expectedKey {
			fmt.Fprintf(input.Out, "  %s Hive secret %s/%s contains the new key\n", colorGreen("[OK]"), ss.Namespace, ss.SecretName)
		} else if ss.AccessKeyID != "" && expectedKey != "" {
			fmt.Fprintf(input.Out, "  %s Hive secret %s/%s key does not match newest key for its user\n", colorYellow("[WARN]"), ss.Namespace, ss.SecretName)
			allOK = false
		} else if ss.AccessKeyID != "" {
			fmt.Fprintf(input.Out, "  %s Hive secret %s/%s has key %s\n", colorGreen("[OK]"), ss.Namespace, ss.SecretName, truncateKeyID(ss.AccessKeyID))
		}
	}

	if allOK {
		fmt.Fprintf(input.Out, "  %s Post-rotation checks passed.\n", colorGreen("[OK]"))
		fmt.Fprintln(input.Out, "\n  Suggested follow-up steps:")
		fmt.Fprintln(input.Out, "  - Remove old access key(s) from the AWS account")
		fmt.Fprintln(input.Out, "  - Verify CCO is healthy:")
		fmt.Fprintln(input.Out, "      oc logs -n openshift-cloud-credential-operator deploy/cloud-credential-operator --tail=50")
		fmt.Fprintln(input.Out, "  - Verify credential secrets are recreated:")
		fmt.Fprintln(input.Out, "      oc get credentialsrequests -A -o wide")
	} else {
		return fmt.Errorf("post-rotation validation found issues — review warnings above")
	}
	return nil
}

// preflightCheckArtifacts validates that all required cluster artifacts exist
// before any AWS keys are created or modified. If a secret is missing, the
// user is prompted to recreate it or abort.
func preflightCheckArtifacts(ctx context.Context, input *RotateSecretInput) error {
	log := getLog(input.Log)

	fmt.Fprintf(input.Out, "Pre-flight: verifying client connections...\n")

	if input.AwsClient == nil {
		return fmt.Errorf("aws client is not connected — cannot proceed with rotation")
	}
	if _, err := input.AwsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{}); err != nil {
		log.WithError(err).Error("AWS client connection check failed")
		return fmt.Errorf("aws client is not responsive: %w", err)
	}
	fmt.Fprintf(input.Out, "  %s AWS client connected\n", colorGreen("[OK]"))

	if input.HiveKubeClient == nil {
		return fmt.Errorf("hive client is not connected — cannot proceed with rotation")
	}
	fmt.Fprintf(input.Out, "  %s Hive client connected\n", colorGreen("[OK]"))

	if input.ManagedClusterClient == nil {
		log.Warn("Managed cluster client is not connected — credential secret deletion will be skipped")
		fmt.Fprintf(input.Out, "  %s Managed cluster client not connected — CR secret deletion will be skipped\n", colorYellow("[WARN]"))
	} else {
		fmt.Fprintf(input.Out, "  %s Managed cluster client connected\n", colorGreen("[OK]"))
	}

	account := input.Account
	ns := account.Spec.ClaimLinkNamespace

	type secretCheck struct {
		name      string
		namespace string
		purpose   string
	}
	var secretsToCheck []secretCheck
	if input.UpdateManagedAdminCreds {
		secretsToCheck = append(secretsToCheck,
			secretCheck{input.AccountCRName + "-secret", awsAccountNamespace, "AAO account secret (stores osdManagedAdmin credentials)"},
			secretCheck{"aws", ns, "Cluster namespace secret (synced to kube-system/aws-creds)"},
		)
	}
	if input.UpdateCcsCreds && account.Spec.BYOC {
		secretsToCheck = append(secretsToCheck,
			secretCheck{"byoc", ns, "CCS admin secret (stores osdCcsAdmin credentials)"},
		)
	}

	fmt.Fprintf(input.Out, "Pre-flight: verifying required Hive secrets exist before rotation...\n")

	for _, sc := range secretsToCheck {
		secret := &corev1.Secret{}
		err := input.HiveKubeClient.Get(ctx, types.NamespacedName{Name: sc.name, Namespace: sc.namespace}, secret)
		if err == nil {
			fmt.Fprintf(input.Out, "  %s %s/%s (%s)\n", colorGreen("[OK]"), sc.namespace, sc.name, sc.purpose)
			continue
		}

		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to check secret %s/%s: %w", sc.namespace, sc.name, err)
		}

		fmt.Fprintf(input.Out, "\n  %s Secret %s/%s not found.\n", colorRed("[MISSING]"), sc.namespace, sc.name)
		fmt.Fprintf(input.Out, "  Purpose: %s\n", sc.purpose)
		fmt.Fprintf(input.Out, "  Verify manually:\n")
		fmt.Fprintf(input.Out, "    oc get secret %s -n %s\n", sc.name, sc.namespace)
		fmt.Fprintf(input.Out, "\n  The rotation tool can create an empty secret that will be populated\n")
		fmt.Fprintf(input.Out, "  with the new credentials during rotation.\n")
		fmt.Fprintf(input.Out, "  Create secret %s/%s? (y/N): ", sc.namespace, sc.name)

		if !confirmFrom(input.In) {
			return fmt.Errorf("required secret %s/%s is missing — rotation aborted", sc.namespace, sc.name)
		}

		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sc.name,
				Namespace: sc.namespace,
			},
			Data: map[string][]byte{},
		}
		if err := input.HiveKubeClient.Create(ctx, newSecret); err != nil {
			return fmt.Errorf("failed to create secret %s/%s: %w", sc.namespace, sc.name, err)
		}
		fmt.Fprintf(input.Out, "  %s Created secret %s/%s\n", colorGreen("[OK]"), sc.namespace, sc.name)
	}

	if input.UpdateManagedAdminCreds {
		cdList := &hiveapiv1.ClusterDeploymentList{}
		if err := input.HiveKubeClient.List(ctx, cdList, client.InNamespace(ns)); err != nil {
			return fmt.Errorf("failed to list ClusterDeployments in %s: %w", ns, err)
		}
		if len(cdList.Items) == 0 {
			return fmt.Errorf("no ClusterDeployments found in %s — cannot create SyncSet", ns)
		}
		fmt.Fprintf(input.Out, "  %s ClusterDeployment found in %s\n", colorGreen("[OK]"), ns)
	}

	fmt.Fprintf(input.Out, "Pre-flight: all required artifacts verified.\n\n")
	return nil
}

// updateSecret fetches an existing k8s secret and replaces its data.
func updateSecret(ctx context.Context, kubeClient client.Client, secretName, secretNamespace string, secretBody map[string][]byte) error {
	secret := &corev1.Secret{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret); err != nil {
		return err
	}
	secret.Data = secretBody
	return kubeClient.Update(ctx, secret)
}

// syncCredentialsToCluster creates a SyncSet to push the "aws" secret to the
// cluster's kube-system namespace, polls ClusterSync for completion, and
// cleans up the SyncSet.
func syncCredentialsToCluster(ctx context.Context, kubeClient client.Client, claimLinkNamespace string, out io.Writer) error {
	clusterDeployments := &hiveapiv1.ClusterDeploymentList{}
	if err := kubeClient.List(ctx, clusterDeployments, client.InNamespace(claimLinkNamespace)); err != nil {
		return err
	}

	if len(clusterDeployments.Items) == 0 {
		return fmt.Errorf("failed to retrieve cluster deployments")
	}
	cdName := clusterDeployments.Items[0].Name

	syncSetName := "aws-sync"
	syncSet := &hiveapiv1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      syncSetName,
			Namespace: claimLinkNamespace,
		},
		Spec: hiveapiv1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{Name: cdName},
			},
			SyncSetCommonSpec: hiveapiv1.SyncSetCommonSpec{
				ResourceApplyMode: "Upsert",
				Secrets: []hiveapiv1.SecretMapping{
					{
						SourceRef: hiveapiv1.SecretReference{
							Name: "aws",
						},
						TargetRef: hiveapiv1.SecretReference{
							Name:      "aws-creds",
							Namespace: "kube-system",
						},
					},
				},
			},
		},
	}

	fmt.Fprintln(out, "Syncing AWS creds down to cluster.")
	if err := kubeClient.Create(ctx, syncSet); apierrors.IsAlreadyExists(err) {
		existing := &hiveapiv1.SyncSet{}
		if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(syncSet), existing); err != nil {
			return err
		}
		syncSet.ResourceVersion = existing.ResourceVersion
		if err := kubeClient.Update(ctx, syncSet); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	fmt.Fprintf(out, "Watching Cluster Sync Status for deployment...")
	if err := hiveinternalv1alpha1.AddToScheme(kubeClient.Scheme()); err != nil {
		return err
	}

	searchStatus := &hiveinternalv1alpha1.ClusterSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cdName,
			Namespace: claimLinkNamespace,
		},
	}
	foundStatus := &hiveinternalv1alpha1.ClusterSync{}
	isSSSynced := false
	for range SyncMaxRetries {
		if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(searchStatus), foundStatus); err != nil {
			// Allow some time to pass before retrying - maybe object creation was slow.
			if apierrors.IsNotFound(err) {
				fmt.Fprintf(out, ".")
				time.Sleep(SyncPollInterval)
				continue
			}
			return err
		}

		for _, status := range foundStatus.Status.SyncSets {
			if status.Name == syncSetName && status.FirstSuccessTime != nil {
				isSSSynced = true
				break
			}
		}

		if isSSSynced {
			fmt.Fprintf(out, "\nSync completed...\n")
			break
		}

		fmt.Fprintf(out, ".")
		time.Sleep(SyncPollInterval)
	}
	if !isSSSynced {
		return fmt.Errorf("syncset failed to sync. Please verify")
	}

	// Clean up the SyncSet
	return kubeClient.Delete(ctx, syncSet)
}

// rotateCcsAdminCredentials rotates the osdCcsAdmin IAM user credentials
// if the account is CCS (BYOC).
func rotateCcsAdminCredentials(ctx context.Context, awsClient awsprovider.Client, kubeClient client.Client, account *awsv1alpha1.Account, report *DiagnosticReport, logger *logrus.Logger, in io.Reader, out io.Writer) error {
	log := getLog(logger)
	if !account.Spec.BYOC {
		fmt.Fprintln(out, "Account is not CCS, skipping osdCcsAdmin credential rotation")
		return nil
	}

	ccsUsername := "osdCcsAdmin"
	createAccessKeyOutput, err := createAccessKeyWithRetry(awsClient, ccsUsername, account.Spec.AwsAccountID, report, in, out)
	if err != nil {
		return err
	}
	if createAccessKeyOutput == nil || createAccessKeyOutput.AccessKey == nil {
		return fmt.Errorf("AWS returned nil access key for %s", ccsUsername)
	}

	ccsNewKeyID := *createAccessKeyOutput.AccessKey.AccessKeyId

	ccsRotationCommitted := false
	defer func() {
		if !ccsRotationCommitted {
			log.Warnf("CCS rotation did not complete — rolling back access key %s", ccsNewKeyID)
			if _, delErr := awsClient.DeleteAccessKey(&iam.DeleteAccessKeyInput{
				UserName:    awsSdk.String(ccsUsername),
				AccessKeyId: awsSdk.String(ccsNewKeyID),
			}); delErr != nil {
				log.WithError(delErr).Errorf("Failed to delete orphaned CCS key %s — manual cleanup required via AWS IAM console", ccsNewKeyID)
			} else {
				log.Infof("Rolled back CCS access key %s", ccsNewKeyID)
			}
		}
	}()

	if err := reportAccessKeys(awsClient, ccsUsername, ccsNewKeyID, out); err != nil {
		return err
	}

	newSecretData := map[string][]byte{
		"aws_user_name":         []byte(*createAccessKeyOutput.AccessKey.UserName),
		"aws_access_key_id":     []byte(ccsNewKeyID),
		"aws_secret_access_key": []byte(*createAccessKeyOutput.AccessKey.SecretAccessKey),
	}

	fmt.Fprintf(out, "Updating Hive secret %s/byoc with new osdCcsAdmin credentials...\n", account.Spec.ClaimLinkNamespace)
	if err := updateSecret(ctx, kubeClient, "byoc", account.Spec.ClaimLinkNamespace, newSecretData); err != nil {
		return fmt.Errorf("failed to update byoc secret: %w", err)
	}
	ccsRotationCommitted = true
	fmt.Fprintf(out, "%s Updated Hive secret %s/byoc\n", colorGreen("[OK]"), account.Spec.ClaimLinkNamespace)
	fmt.Fprintln(out, "Successfully rotated credentials for osdCcsAdmin")
	return nil
}

const credentialRequestNamespace = "openshift-cloud-credential-operator"

// dryRunDeleteCredentialSecrets lists the secrets referenced by AWS
// CredentialRequests that would be deleted during a real rotation,
// and verifies access + RBAC for each.
func dryRunDeleteCredentialSecrets(ctx context.Context, managedClient client.Client, out io.Writer, dr *dryRunChecker) error {
	crList := &ccov1.CredentialsRequestList{}
	if err := managedClient.List(ctx, crList); err != nil {
		dr.report(false, "Managed cluster: LIST CredentialRequests (all namespaces)")
		return fmt.Errorf("failed to list CredentialRequests: %w", err)
	}
	dr.report(true, "Managed cluster: LIST CredentialRequests (all namespaces)")

	count := 0
	for i := range crList.Items {
		cr := &crList.Items[i]
		if !isAWSCredentialRequestForDiagnostic(cr) {
			continue
		}
		ref := cr.Spec.SecretRef
		fmt.Fprintf(out, "%s %s\n", colorBlue("[Dry Run]"),
			colorBlue(fmt.Sprintf("Would delete secret %s/%s (referenced by CredentialRequest %s)", ref.Namespace, ref.Name, cr.Name)))

		s := &corev1.Secret{}
		getErr := managedClient.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, s)
		if getErr != nil {
			if apierrors.IsNotFound(getErr) {
				dr.info("Managed cluster: secret %s/%s does not exist — delete will be skipped, CCO recreates after rotation", ref.Namespace, ref.Name)
			} else {
				dr.report(false, "Managed cluster: GET secret %s/%s", ref.Namespace, ref.Name)
			}
		} else {
			dr.report(true, "Managed cluster: GET secret %s/%s", ref.Namespace, ref.Name)
			dr.checkCanI(ctx, managedClient, "Managed cluster", "delete", "secrets", "", ref.Namespace)
		}
		count++
	}
	fmt.Fprintf(out, "%s %s\n", colorBlue("[Dry Run]"), colorBlue(fmt.Sprintf("Would delete %d credential secret(s) total", count)))
	return nil
}

// CRSecretPollInterval is the delay between CR health checks after deleting a secret.
var CRSecretPollInterval = 5 * time.Second

// CRSecretPollTimeout is the maximum time to wait for CCO to recreate a secret.
var CRSecretPollTimeout = 60 * time.Second

// DeleteCredentialSecrets sequentially deletes each credential secret and
// waits for CCO to recreate it before proceeding to the next one.
func DeleteCredentialSecrets(ctx context.Context, managedClient client.Client, in io.Reader, out io.Writer) error {
	fmt.Fprintln(out, "The 'aws-creds' secret in 'kube-system' has been updated via SyncSet.")
	fmt.Fprintln(out, "Deleting credential secrets one at a time, verifying CCO recreates each before continuing...")

	crList := &ccov1.CredentialsRequestList{}
	if err := managedClient.List(ctx, crList); err != nil {
		return fmt.Errorf("failed to list CredentialRequests: %w", err)
	}

	var awsCRs []*ccov1.CredentialsRequest
	for i := range crList.Items {
		cr := &crList.Items[i]
		if isAWSCredentialRequestForDiagnostic(cr) {
			awsCRs = append(awsCRs, cr)
		}
	}

	deletedCount := 0
	for idx, cr := range awsCRs {
		ref := cr.Spec.SecretRef
		fmt.Fprintf(out, "\n  [%d/%d] CredentialRequest: %s\n", idx+1, len(awsCRs), cr.Name)

		// Record the existing secret's UID so we can detect a genuine recreation
		existingSecret := &corev1.Secret{}
		var oldUID types.UID
		if err := managedClient.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, existingSecret); err != nil {
			if apierrors.IsNotFound(err) {
				fmt.Fprintf(out, "    Secret %s/%s already absent, waiting for CCO to create it...\n", ref.Namespace, ref.Name)
				recreated := waitForCRSecret(ctx, managedClient, cr, oldUID, out)
				if recreated {
					fmt.Fprintf(out, "    %s Secret recreated by CCO\n", colorGreen("[OK]"))
				}
				continue
			}
			return fmt.Errorf("failed to read secret %s/%s: %w", ref.Namespace, ref.Name, err)
		}
		oldUID = existingSecret.UID

		if err := managedClient.Delete(ctx, existingSecret); err != nil {
			if apierrors.IsNotFound(err) {
				fmt.Fprintf(out, "    Secret %s/%s deleted between read and delete, waiting for CCO...\n", ref.Namespace, ref.Name)
			} else {
				return fmt.Errorf("failed to delete secret %s/%s: %w", ref.Namespace, ref.Name, err)
			}
		} else {
			fmt.Fprintf(out, "    Deleted secret %s/%s\n", ref.Namespace, ref.Name)
		}
		deletedCount++

		if idx == 0 {
			fmt.Fprintf(out, "    Waiting for CCO to recreate secret (first secret — validating CCO health)...\n")
		} else {
			fmt.Fprintf(out, "    Waiting for CCO to recreate secret...\n")
		}

		recreated := waitForCRSecret(ctx, managedClient, cr, oldUID, out)
		if !recreated {
			fmt.Fprintf(out, "\n    %s Secret %s/%s was not recreated within %s.\n", colorYellow("[TIMEOUT]"), ref.Namespace, ref.Name, CRSecretPollTimeout)
			fmt.Fprintf(out, "    Check CCO health: oc logs -n openshift-cloud-credential-operator deploy/cloud-credential-operator\n")
			fmt.Fprintf(out, "    Check CR status:  oc get credentialsrequest %s -n %s -o yaml\n", cr.Name, cr.Namespace)

			remaining := len(awsCRs) - idx - 1
			if remaining > 0 {
				fmt.Fprintf(out, "\n    %d credential secret(s) remaining to delete.\n", remaining)
				fmt.Fprintf(out, "    Continue deleting remaining secrets? (y/N): ")
				if !confirmFrom(in) {
					fmt.Fprintf(out, "\n    Operation paused. %d of %d secret(s) deleted so far.\n", deletedCount, len(awsCRs))
					fmt.Fprintln(out, "\n    To complete the rotation manually, delete the remaining credential secrets:")
					for _, remaining := range awsCRs[idx+1:] {
						rRef := remaining.Spec.SecretRef
						fmt.Fprintf(out, "      oc delete secret %s -n %s\n", rRef.Name, rRef.Namespace)
					}
					fmt.Fprintln(out, "\n    Then verify all secrets are recreated:")
					fmt.Fprintln(out, "      oc get credentialsrequests -A -o wide")
					return nil
				}
			}
		} else {
			fmt.Fprintf(out, "    %s Secret %s/%s recreated by CCO\n", colorGreen("[OK]"), ref.Namespace, ref.Name)
		}
	}

	fmt.Fprintf(out, "\nDeleted and verified %d credential secret(s).\n", deletedCount)
	return nil
}

// waitForCRSecret polls until CCO recreates the secret referenced by a CredentialRequest
// (detected by a new UID and Provisioned status), returning false on timeout.
func waitForCRSecret(ctx context.Context, managedClient client.Client, cr *ccov1.CredentialsRequest, oldUID types.UID, out io.Writer) bool {
	ref := cr.Spec.SecretRef
	deadline := time.Now().Add(CRSecretPollTimeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			fmt.Fprintf(out, "\n")
			return false
		case <-time.After(CRSecretPollInterval):
		}

		secret := &corev1.Secret{}
		if err := managedClient.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, secret); err != nil {
			fmt.Fprintf(out, ".")
			continue
		}

		// If we have an old UID, ensure this is a new object (not the pre-delete one still visible)
		if oldUID != "" && secret.UID == oldUID {
			fmt.Fprintf(out, ".")
			continue
		}

		updatedCR := &ccov1.CredentialsRequest{}
		if err := managedClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, updatedCR); err != nil {
			fmt.Fprintf(out, ".")
			continue
		}

		if updatedCR.Status.Provisioned {
			hasFailed := false
			for _, cond := range updatedCR.Status.Conditions {
				if cond.Type == ccov1.CredentialsProvisionFailure && cond.Status == corev1.ConditionTrue {
					hasFailed = true
					fmt.Fprintf(out, "\n    %s CR %s has CredentialsProvisionFailure: %s\n", colorRed("[WARN]"), cr.Name, cond.Message)
				}
			}
			if !hasFailed {
				fmt.Fprintf(out, "\n")
				return true
			}
		}
		fmt.Fprintf(out, ".")
	}
	fmt.Fprintf(out, "\n")
	return false
}

type dryRunChecker struct {
	out   io.Writer
	allOK bool
}

func (d *dryRunChecker) would(format string, args ...any) {
	fmt.Fprintf(d.out, "%s %s %s\n", colorBlue("[Dry Run]"), colorBlue("Would"), colorBlue(fmt.Sprintf(format, args...)))
}

func (d *dryRunChecker) report(ok bool, format string, args ...any) {
	status := colorGreen("[OK]")
	if !ok {
		status = colorRed("[FAIL]")
		d.allOK = false
	}
	fmt.Fprintf(d.out, "%s %s %s\n", colorBlue("[Dry Run]"), status, fmt.Sprintf(format, args...))
}

// checkCanI performs a SelfSubjectAccessReview to verify the caller has RBAC
// permission for the given verb on the resource in the namespace.
func (d *dryRunChecker) checkCanI(ctx context.Context, k8sClient client.Client, label, verb, resource, group, namespace string) {
	ssar := &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{
				Namespace: namespace,
				Verb:      verb,
				Group:     group,
				Resource:  resource,
			},
		},
	}

	_ = authv1.AddToScheme(k8sClient.Scheme())

	if err := k8sClient.Create(ctx, ssar); err != nil {
		fmt.Fprintf(d.out, "%s %s %s: auth can-i %s %s in %s (could not verify: %v)\n",
			colorBlue("[Dry Run]"), colorYellow("[SKIP]"), label, verb, resource, namespace, err)
		return
	}

	if !ssar.Status.Allowed {
		reason := ssar.Status.Reason
		if reason == "" {
			reason = "denied by RBAC"
		}
		d.report(false, "%s: auth can-i %s %s in %s (%s)", label, verb, resource, namespace, reason)
	} else {
		d.report(true, "%s: auth can-i %s %s in %s", label, verb, resource, namespace)
	}
}

// checkResourceAccess combines a GET (resource exists + readable) with a
// SelfSubjectAccessReview for the target verb.
func (d *dryRunChecker) checkResourceAccess(ctx context.Context, k8sClient client.Client, label, verb, resource, namespace, name string) {
	obj := &corev1.Secret{}
	getErr := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj)
	if getErr != nil {
		if apierrors.IsNotFound(getErr) {
			d.report(false, "%s: secret %s/%s not found — rotation requires this secret to exist", label, namespace, name)
		} else {
			d.report(false, "%s: GET secret %s/%s (%v)", label, namespace, name, getErr)
		}
		return
	}
	d.report(true, "%s: GET secret %s/%s", label, namespace, name)
	d.checkCanI(ctx, k8sClient, label, verb, resource, "", namespace)
}

func (d *dryRunChecker) info(format string, args ...any) {
	fmt.Fprintf(d.out, "%s %s %s\n", colorBlue("[Dry Run]"), colorBlue("[INFO]"), colorBlue(fmt.Sprintf(format, args...)))
}
