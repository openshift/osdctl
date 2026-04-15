package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	hiveapiv1 "github.com/openshift/hive/apis/hive/v1"
	hiveinternalv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	awsAccountNamespace = "aws-account-operator"
	osdManagedAdminIAM  = "osdManagedAdmin"
)

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
		return fmt.Errorf("Account %s is STS - No IAM User Credentials to Rotate", input.AccountCRName)
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

	newSecretData := map[string][]byte{
		"aws_user_name":         []byte(*createAccessKeyOutput.AccessKey.UserName),
		"aws_access_key_id":     []byte(*createAccessKeyOutput.AccessKey.AccessKeyId),
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
		return fmt.Errorf("insufficient permissions for secret rotation. Denied actions: %v", deniedActions)
	}

	fmt.Fprintln(out, "Permission verification successful. All required IAM actions are allowed.")
	return nil
}

// resolveAdminUsername verifies rotation permissions for the given username,
// falling back to the unsuffixed osdManagedAdmin if needed.
func resolveAdminUsername(out io.Writer, awsClient awsprovider.Client, accountID, username, suffix string) (string, error) {
	err := VerifyRotationPermissions(out, awsClient, accountID, username)
	if err != nil {
		// If the suffixed username failed, try without the suffix
		if username == osdManagedAdminIAM+"-"+suffix {
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
		return fmt.Errorf("failed to retreive cluster deployments")
	}
	cdName := clusterDeployments.Items[0].ObjectMeta.Name

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
	if err := kubeClient.Create(ctx, syncSet); err != nil {
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

	createAccessKeyOutput, err := awsClient.CreateAccessKey(&iam.CreateAccessKeyInput{
		UserName: awsSdk.String("osdCcsAdmin"),
	})
	if err != nil {
		return err
	}

	newSecretData := map[string][]byte{
		"aws_user_name":         []byte(*createAccessKeyOutput.AccessKey.UserName),
		"aws_access_key_id":     []byte(*createAccessKeyOutput.AccessKey.AccessKeyId),
		"aws_secret_access_key": []byte(*createAccessKeyOutput.AccessKey.SecretAccessKey),
	}

	if err := updateSecret(ctx, kubeClient, "byoc", account.Spec.ClaimLinkNamespace, newSecretData); err != nil {
		return err
	}

	fmt.Fprintln(out, "Successfully rotated secrets for osdCcsAdmin")
	return nil
}
