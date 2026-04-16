package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	ccov1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	hiveapiv1 "github.com/openshift/hive/apis/hive/v1"
	hiveinternalv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
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

// InsufficientPermissionsError is returned when SimulatePrincipalPolicy
// reports that one or more required IAM actions are denied.
type InsufficientPermissionsError struct {
	DeniedActions []string
}

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

	// Out is the writer for informational output.
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
	account := input.Account

	if account.Spec.ManualSTSMode {
		return fmt.Errorf("account %s is STS - No IAM User Credentials to Rotate", input.AccountCRName)
	}

	accountID := account.Spec.AwsAccountID

	accountIDSuffixLabel, ok := account.Labels["iamUserId"]
	if !ok {
		return fmt.Errorf("no label on Account CR for IAM User")
	}

	// Resolve the admin username, with fallback from suffixed to unsuffixed
	adminUsername := input.OsdManagedAdminUsername
	if adminUsername == "" {
		adminUsername = osdManagedAdminIAM + "-" + accountIDSuffixLabel
	}

	adminUsername, err := resolveAdminUsername(input.Out, input.AwsClient, accountID, adminUsername, accountIDSuffixLabel)
	if err != nil {
		return err
	}

	if input.DryRun {
		fmt.Fprintln(input.Out, "[Dry Run] Would create a new IAM access key for user:", adminUsername)
		fmt.Fprintln(input.Out, "[Dry Run] Would list access keys and report old keys to remove via rh-aws-saml-login")
		fmt.Fprintf(input.Out, "[Dry Run] Would update secret %s/%s with new credentials\n", awsAccountNamespace, input.AccountCRName+"-secret")
		fmt.Fprintf(input.Out, "[Dry Run] Would update secret %s/%s with new credentials\n", account.Spec.ClaimLinkNamespace, "aws")
		fmt.Fprintf(input.Out, "[Dry Run] Would create SyncSet %s/%s to sync credentials to cluster\n", account.Spec.ClaimLinkNamespace, "aws-sync")
		fmt.Fprintf(input.Out, "[Dry Run] Would poll ClusterSync and then delete SyncSet %s/%s\n", account.Spec.ClaimLinkNamespace, "aws-sync")

		if err := dryRunDeleteCredentialSecrets(ctx, input.ManagedClusterClient, input.Out); err != nil {
			return err
		}

		if input.UpdateCcsCreds {
			if account.Spec.BYOC {
				fmt.Fprintln(input.Out, "[Dry Run] Would create a new IAM access key for user: osdCcsAdmin")
				fmt.Fprintf(input.Out, "[Dry Run] Would update secret %s/%s with new osdCcsAdmin credentials\n", account.Spec.ClaimLinkNamespace, "byoc")
			} else {
				fmt.Fprintln(input.Out, "[Dry Run] Account is not CCS, would skip osdCcsAdmin credential rotation")
			}
		}

		fmt.Fprintln(input.Out, "[Dry Run] No changes were made.")
		return nil
	}

	// Create new access key
	createAccessKeyOutput, err := input.AwsClient.CreateAccessKey(&iam.CreateAccessKeyInput{
		UserName: awsSdk.String(adminUsername),
	})
	if err != nil {
		var nse *iamTypes.NoSuchEntityException
		if errors.As(err, &nse) {
			// Retry without the suffix
			adminUsername = osdManagedAdminIAM
			createAccessKeyOutput, err = input.AwsClient.CreateAccessKey(&iam.CreateAccessKeyInput{
				UserName: awsSdk.String(adminUsername),
			})
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	newKeyID := *createAccessKeyOutput.AccessKey.AccessKeyId

	if err := reportAccessKeys(input.AwsClient, adminUsername, newKeyID, input.Out); err != nil {
		return err
	}

	newSecretData := map[string][]byte{
		"aws_user_name":         []byte(*createAccessKeyOutput.AccessKey.UserName),
		"aws_access_key_id":     []byte(newKeyID),
		"aws_secret_access_key": []byte(*createAccessKeyOutput.AccessKey.SecretAccessKey),
	}

	// Update the account secret
	if err := updateSecret(ctx, input.HiveKubeClient, input.AccountCRName+"-secret", awsAccountNamespace, newSecretData); err != nil {
		return err
	}

	// Update the secret in ClusterDeployment's namespace
	if err := updateSecret(ctx, input.HiveKubeClient, "aws", account.Spec.ClaimLinkNamespace, newSecretData); err != nil {
		return err
	}

	fmt.Fprintln(input.Out, "AWS creds updated on hive.")

	if err := syncCredentialsToCluster(ctx, input.HiveKubeClient, account.Spec.ClaimLinkNamespace, input.Out); err != nil {
		return err
	}

	fmt.Fprintf(input.Out, "Successfully rotated secrets for %s\n", adminUsername)

	// Delete the secrets referenced by AWS CredentialRequests so CCO recreates
	// them with the newly synced credentials.
	if err := deleteCredentialSecrets(ctx, input.ManagedClusterClient, input.Out); err != nil {
		return err
	}

	if input.UpdateCcsCreds {
		if err := rotateCcsAdminCredentials(ctx, input.AwsClient, input.HiveKubeClient, account, input.Out); err != nil {
			return err
		}
	}

	return nil
}

// VerifyRotationPermissions checks if the assumed role has the necessary IAM
// permissions to perform secret rotation by simulating the required actions.
func VerifyRotationPermissions(out io.Writer, awsClient awsprovider.Client, accountID string, username string) error {
	requiredActions := []string{
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

	userArn := fmt.Sprintf("arn:aws:iam::%s:user/%s", accountID, username)

	fmt.Fprintf(out, "Verifying IAM permissions for user %s...\n", username)

	output, err := awsClient.SimulatePrincipalPolicy(&iam.SimulatePrincipalPolicyInput{
		PolicySourceArn: awsSdk.String(userArn),
		ActionNames:     requiredActions,
	})
	if err != nil {
		return fmt.Errorf("failed to simulate principal policy: %w", err)
	}

	var deniedActions []string
	for _, result := range output.EvaluationResults {
		if result.EvalDecision != iamTypes.PolicyEvaluationDecisionTypeAllowed {
			deniedActions = append(deniedActions, *result.EvalActionName)
		}
	}

	if len(deniedActions) > 0 {
		return &InsufficientPermissionsError{DeniedActions: deniedActions}
	}

	fmt.Fprintln(out, "Permission verification successful. All required IAM actions are allowed.")
	return nil
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
		if *key.AccessKeyId == newKeyID {
			fmt.Fprintf(out, "  - %s (new - just created)\n", *key.AccessKeyId)
		} else {
			fmt.Fprintf(out, "  - %s (old - should be removed)\n", *key.AccessKeyId)
		}
	}

	hasOldKeys := false
	for _, key := range listOutput.AccessKeyMetadata {
		if *key.AccessKeyId != newKeyID {
			hasOldKeys = true
			break
		}
	}
	if hasOldKeys {
		fmt.Fprintf(out, "\nThe old access key(s) listed above should now be removed.\n")
		fmt.Fprintf(out, "Use 'rh-aws-saml-login' to gain access to the account and delete them.\n\n")
	}

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
func rotateCcsAdminCredentials(ctx context.Context, awsClient awsprovider.Client, kubeClient client.Client, account *awsv1alpha1.Account, out io.Writer) error {
	if !account.Spec.BYOC {
		fmt.Fprintln(out, "Account is not CCS, skipping osdCcsAdmin credential rotation")
		return nil
	}

	ccsUsername := "osdCcsAdmin"
	createAccessKeyOutput, err := awsClient.CreateAccessKey(&iam.CreateAccessKeyInput{
		UserName: awsSdk.String(ccsUsername),
	})
	if err != nil {
		return err
	}

	ccsNewKeyID := *createAccessKeyOutput.AccessKey.AccessKeyId

	if err := reportAccessKeys(awsClient, ccsUsername, ccsNewKeyID, out); err != nil {
		return err
	}

	newSecretData := map[string][]byte{
		"aws_user_name":         []byte(*createAccessKeyOutput.AccessKey.UserName),
		"aws_access_key_id":     []byte(ccsNewKeyID),
		"aws_secret_access_key": []byte(*createAccessKeyOutput.AccessKey.SecretAccessKey),
	}

	if err := updateSecret(ctx, kubeClient, "byoc", account.Spec.ClaimLinkNamespace, newSecretData); err != nil {
		return err
	}

	fmt.Fprintln(out, "Successfully rotated secrets for osdCcsAdmin")
	return nil
}

const (
	credentialRequestNamespace = "openshift-cloud-credential-operator"
	credentialRequestPrefix    = "openshift-"
)

// isAWSCredentialRequest returns true when the CredentialRequest has the
// "openshift-" name prefix and its providerSpec.kind is "AWSProviderSpec".
func isAWSCredentialRequest(cr *ccov1.CredentialsRequest) bool {
	if !strings.HasPrefix(cr.Name, credentialRequestPrefix) {
		return false
	}
	if cr.Spec.ProviderSpec == nil || cr.Spec.ProviderSpec.Raw == nil {
		return false
	}
	var provider struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(cr.Spec.ProviderSpec.Raw, &provider); err != nil {
		return false
	}
	return provider.Kind == "AWSProviderSpec"
}

// dryRunDeleteCredentialSecrets lists the secrets referenced by AWS
// CredentialRequests that would be deleted during a real rotation.
func dryRunDeleteCredentialSecrets(ctx context.Context, managedClient client.Client, out io.Writer) error {
	crList := &ccov1.CredentialsRequestList{}
	if err := managedClient.List(ctx, crList, client.InNamespace(credentialRequestNamespace)); err != nil {
		return fmt.Errorf("failed to list CredentialRequests in %s: %w", credentialRequestNamespace, err)
	}

	count := 0
	for i := range crList.Items {
		cr := &crList.Items[i]
		if !isAWSCredentialRequest(cr) {
			continue
		}
		fmt.Fprintf(out, "[Dry Run] Would delete secret %s/%s (referenced by CredentialRequest %s)\n",
			cr.Spec.SecretRef.Namespace, cr.Spec.SecretRef.Name, cr.Name)
		count++
	}
	fmt.Fprintf(out, "[Dry Run] Would delete %d credential secret(s) total\n", count)
	return nil
}

// deleteCredentialSecrets lists AWS CredentialRequests on the managed cluster,
// resolves the secret each one references via .spec.secretRef, and deletes
// those secrets so CCO recreates them using the newly synced credentials.
func deleteCredentialSecrets(ctx context.Context, managedClient client.Client, out io.Writer) error {
	fmt.Fprintln(out, "The 'aws-creds' secret in 'kube-system' has been updated via SyncSet.")
	fmt.Fprintln(out, "Deleting credential secrets so CCO recreates them with the new credentials...")

	crList := &ccov1.CredentialsRequestList{}
	if err := managedClient.List(ctx, crList, client.InNamespace(credentialRequestNamespace)); err != nil {
		return fmt.Errorf("failed to list CredentialRequests in %s: %w", credentialRequestNamespace, err)
	}

	deletedCount := 0
	for i := range crList.Items {
		cr := &crList.Items[i]
		if !isAWSCredentialRequest(cr) {
			continue
		}
		ref := cr.Spec.SecretRef
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ref.Name,
				Namespace: ref.Namespace,
			},
		}
		if err := managedClient.Delete(ctx, secret); err != nil {
			if apierrors.IsNotFound(err) {
				fmt.Fprintf(out, "  Secret %s/%s already absent, skipping\n", ref.Namespace, ref.Name)
				continue
			}
			return fmt.Errorf("failed to delete secret %s/%s (from CredentialRequest %s): %w", ref.Namespace, ref.Name, cr.Name, err)
		}
		fmt.Fprintf(out, "  Deleted secret %s/%s (referenced by CredentialRequest %s)\n", ref.Namespace, ref.Name, cr.Name)
		deletedCount++
	}

	fmt.Fprintf(out, "Deleted %d credential secret(s). CCO will recreate them with the updated credentials.\n", deletedCount)
	return nil
}
