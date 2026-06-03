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
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/fatih/color"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	ccov1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	"github.com/openshift/osdctl/pkg/policies"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AWSCredsInput holds all resolved dependencies needed for both diagnostics
// and rotation. The CLI layer resolves AWS and k8s clients before calling
// DiagnoseCredentials or RotateCredentials.
type AWSCredsInput struct {
	ClusterID         string
	ClusterName       string
	ClusterExternalID string
	IsCCS             bool
	AWSAccountID      string
	AccountCRName     string
	Account           *awsv1alpha1.Account
	AdminUsername     string
	AwsClient         awsprovider.Client
	HiveKubeClient    client.Client
	ManagedClient     client.Client
	Log               *logrus.Logger
	Out               io.Writer
}

type KeyStatus struct {
	UserName    string
	AccessKeyID string
	Age         time.Duration
	CreateDate  time.Time
	Status      string
	HiveMatch   bool
}

type SecretStatus struct {
	SecretName   string
	Namespace    string
	AccessKeyID  string
	MatchesAWS   bool
	Exists       bool
	ErrorMessage string
}

type CredRequestStatus struct {
	CredRequestName string
	SecretName      string
	Namespace       string
	Age             time.Duration
	Exists          bool
	NeedsRecreation bool
	ErrorMessage    string
}

type PermissionResult struct {
	Action      string
	Allowed     bool
	Category    string   // "rotation" or "credreq"
	RequestedBy []string // CR names that request this action (credreq category only)
}

type Finding struct {
	Severity string // "OK", "WARN", "FAIL"
	Message  string
	Guidance string
}

type DiagnosticReport struct {
	ClusterID         string
	ClusterName       string
	ClusterExternalID string
	IsCCS             bool
	AWSAccountID      string
	AccountCRName     string

	ManagedAdminUser string
	CcsAdminUser     string
	CallerARN        string
	CallerAccount    string

	Keys         []KeyStatus
	Secrets      []SecretStatus
	CredRequests []CredRequestStatus
	Permissions  []PermissionResult
	Findings     []Finding

	AllPermissionsOK bool
	AllSecretsInSync bool
}

func DiagnoseCredentials(ctx context.Context, input *AWSCredsInput) (*DiagnosticReport, error) {
	report := &DiagnosticReport{
		ClusterID:         input.ClusterID,
		ClusterName:       input.ClusterName,
		ClusterExternalID: input.ClusterExternalID,
		IsCCS:             input.IsCCS,
		AWSAccountID:      input.AWSAccountID,
		AccountCRName:     input.AccountCRName,
		ManagedAdminUser:  input.AdminUsername,
		AllPermissionsOK:  true,
		AllSecretsInSync:  true,
	}

	if input.IsCCS {
		report.CcsAdminUser = "osdCcsAdmin"
	}

	log := getLog(input.Log)

	callerIdentity, err := input.AwsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err == nil && callerIdentity != nil {
		if callerIdentity.Arn != nil {
			report.CallerARN = *callerIdentity.Arn
		}
		if callerIdentity.Account != nil {
			report.CallerAccount = *callerIdentity.Account
		}
	} else {
		log.WithError(err).Warn("Could not determine AWS caller identity")
		fmt.Fprintf(input.Out, "  %s Could not determine AWS caller identity: %v\n", colorYellow("[WARN]"), err)
	}

	hiveSecretKeyIDs := map[string]string{}

	if err := diagnoseHiveSecrets(ctx, input, report, hiveSecretKeyIDs); err != nil {
		return nil, fmt.Errorf("hive secret diagnostics failed: %w", err)
	}

	if err := diagnoseIAMKeys(input, report, hiveSecretKeyIDs); err != nil {
		return nil, fmt.Errorf("IAM key diagnostics failed: %w", err)
	}

	if err := diagnoseCredentialRequests(ctx, input, report); err != nil {
		return nil, fmt.Errorf("credential request diagnostics failed: %w", err)
	}

	if err := diagnosePermissions(ctx, input, report); err != nil {
		return nil, fmt.Errorf("IAM permission check failed: %w", err)
	}

	return report, nil
}

func diagnoseHiveSecrets(ctx context.Context, input *AWSCredsInput, report *DiagnosticReport, keyIDs map[string]string) error {
	secretChecks := []struct {
		name      string
		namespace string
	}{
		{input.AccountCRName + "-secret", awsAccountNamespace},
		{"aws", input.Account.Spec.ClaimLinkNamespace},
	}

	if input.IsCCS {
		secretChecks = append(secretChecks, struct {
			name      string
			namespace string
		}{"byoc", input.Account.Spec.ClaimLinkNamespace})
	}

	for _, sc := range secretChecks {
		ss := SecretStatus{
			SecretName: sc.name,
			Namespace:  sc.namespace,
		}

		secret := &corev1.Secret{}
		err := input.HiveKubeClient.Get(ctx, types.NamespacedName{
			Name:      sc.name,
			Namespace: sc.namespace,
		}, secret)

		if err != nil {
			if apierrors.IsNotFound(err) {
				ss.Exists = false
				ss.ErrorMessage = "not found"
				report.AllSecretsInSync = false
				report.Findings = append(report.Findings, Finding{
					Severity: "FAIL",
					Message:  fmt.Sprintf("Hive secret %s/%s not found", sc.namespace, sc.name),
					Guidance: fmt.Sprintf("Secret may need to be recreated. Check hive namespace %s for the expected secret.", sc.namespace),
				})
			} else {
				ss.ErrorMessage = err.Error()
				report.AllSecretsInSync = false
				report.Findings = append(report.Findings, Finding{
					Severity: "FAIL",
					Message:  fmt.Sprintf("Hive secret %s/%s read failed: %v", sc.namespace, sc.name, err),
					Guidance: "Could not read the secret — this may be a transient API error or RBAC issue, not a missing secret.",
				})
			}
			report.Secrets = append(report.Secrets, ss)
			continue
		}

		ss.Exists = true
		if keyID, ok := secret.Data["aws_access_key_id"]; ok {
			ss.AccessKeyID = string(keyID)
			keyIDs[sc.name] = string(keyID)
		}

		report.Secrets = append(report.Secrets, ss)
	}

	return nil
}

func diagnoseIAMKeys(input *AWSCredsInput, report *DiagnosticReport, hiveSecretKeyIDs map[string]string) error {
	accountSecretKeyID := hiveSecretKeyIDs[input.AccountCRName+"-secret"]

	adminUser := input.AdminUsername
	listOutput, err := input.AwsClient.ListAccessKeys(&iam.ListAccessKeysInput{
		UserName: awsSdk.String(adminUser),
	})
	if err != nil && isNoSuchEntity(err) && adminUser != osdManagedAdminIAM {
		fmt.Fprintf(input.Out, "  IAM user %s not found, trying %s...\n", adminUser, osdManagedAdminIAM)
		adminUser = osdManagedAdminIAM
		listOutput, err = input.AwsClient.ListAccessKeys(&iam.ListAccessKeysInput{
			UserName: awsSdk.String(adminUser),
		})
	}
	if err != nil {
		if isNoSuchEntity(err) {
			report.Findings = append(report.Findings, Finding{
				Severity: "FAIL",
				Message:  fmt.Sprintf("IAM user %s not found in AWS account %s", adminUser, input.AWSAccountID),
				Guidance: "Neither the suffixed nor unsuffixed osdManagedAdmin user exists.\n       Use --admin-username to specify the correct IAM username.",
			})
		} else {
			return fmt.Errorf("failed to list access keys for %s: %w", adminUser, err)
		}
	}
	report.ManagedAdminUser = adminUser

	users := []string{adminUser}
	if input.IsCCS {
		users = append(users, "osdCcsAdmin")
	}

	for _, username := range users {
		var keyList *iam.ListAccessKeysOutput
		if username == adminUser {
			if listOutput == nil {
				continue
			}
			keyList = listOutput
		} else {
			keyList, err = input.AwsClient.ListAccessKeys(&iam.ListAccessKeysInput{
				UserName: awsSdk.String(username),
			})
			if err != nil {
				if isNoSuchEntity(err) {
					report.Findings = append(report.Findings, Finding{
						Severity: "FAIL",
						Message:  fmt.Sprintf("IAM user %s does not exist in AWS account %s", username, input.AWSAccountID),
						Guidance: "Verify the correct username. Check the Account CR iamUserId label.",
					})
					continue
				}
				return fmt.Errorf("failed to list access keys for %s: %w", username, err)
			}
		}

		var matchKeyID string
		if username == "osdCcsAdmin" {
			matchKeyID = hiveSecretKeyIDs["byoc"]
		} else {
			matchKeyID = accountSecretKeyID
		}

		for _, key := range keyList.AccessKeyMetadata {
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
			}
			if matchKeyID != "" && *key.AccessKeyId == matchKeyID {
				ks.HiveMatch = true
			}
			report.Keys = append(report.Keys, ks)
		}

		keyCount := len(keyList.AccessKeyMetadata)
		if keyCount >= 2 {
			msg := fmt.Sprintf("%s has %d access keys (max 2). Review which key should be deleted before rotation.", username, keyCount)
			report.Findings = append(report.Findings, Finding{
				Severity: "INFO",
				Message:  msg,
			})
		}

		hasMatch := false
		for _, key := range keyList.AccessKeyMetadata {
			if matchKeyID != "" && *key.AccessKeyId == matchKeyID {
				hasMatch = true
				break
			}
		}
		if matchKeyID != "" && !hasMatch {
			report.AllSecretsInSync = false
			report.Findings = append(report.Findings, Finding{
				Severity: "FAIL",
				Message:  fmt.Sprintf("Hive secret key ID %s for user %s does not match any active AWS access key", truncateKeyID(matchKeyID), username),
				Guidance: fmt.Sprintf("Credentials are out of sync. Rotate to fix:\n       osdctl account aws-creds %s --reason <ticket> --rotate-managed-admin", report.ClusterID),
			})
		}
	}

	for i := range report.Secrets {
		ss := &report.Secrets[i]
		if !ss.Exists || ss.AccessKeyID == "" {
			continue
		}
		found := false
		for _, ks := range report.Keys {
			if ks.AccessKeyID == ss.AccessKeyID {
				found = true
				ss.MatchesAWS = true
				break
			}
		}
		if !found {
			ss.MatchesAWS = false
			report.AllSecretsInSync = false
		}
	}

	return nil
}

func diagnoseCredentialRequests(ctx context.Context, input *AWSCredsInput, report *DiagnosticReport) error {
	if input.ManagedClient == nil {
		return nil
	}

	crList := &ccov1.CredentialsRequestList{}
	if err := input.ManagedClient.List(ctx, crList); err != nil {
		report.Findings = append(report.Findings, Finding{
			Severity: "WARN",
			Message:  fmt.Sprintf("Failed to list CredentialRequests: %v", err),
			Guidance: "Ensure you have access to the managed cluster. Try: oc get credentialsrequests -A",
		})
		return nil
	}

	var managedAdminKeyAge time.Duration
	for _, k := range report.Keys {
		if k.HiveMatch && k.UserName != "osdCcsAdmin" {
			managedAdminKeyAge = k.Age
			break
		}
	}

	for i := range crList.Items {
		cr := &crList.Items[i]
		if !isAWSCredReqForDiag(cr) {
			continue
		}

		crs := CredRequestStatus{
			CredRequestName: cr.Name,
			SecretName:      cr.Spec.SecretRef.Name,
			Namespace:       cr.Spec.SecretRef.Namespace,
		}

		secret := &corev1.Secret{}
		err := input.ManagedClient.Get(ctx, types.NamespacedName{
			Name:      cr.Spec.SecretRef.Name,
			Namespace: cr.Spec.SecretRef.Namespace,
		}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				crs.Exists = false
				crs.ErrorMessage = "not found"
				report.Findings = append(report.Findings, Finding{
					Severity: "WARN",
					Message:  fmt.Sprintf("Credential secret %s/%s (from CredentialRequest %s) not found", cr.Spec.SecretRef.Namespace, cr.Spec.SecretRef.Name, cr.Name),
					Guidance: "CCO should recreate this secret. If it persists, check CCO logs:\n       oc logs -n openshift-cloud-credential-operator deploy/cloud-credential-operator",
				})
			} else {
				crs.ErrorMessage = err.Error()
				report.Findings = append(report.Findings, Finding{
					Severity: "WARN",
					Message:  fmt.Sprintf("Credential secret %s/%s read failed: %v", cr.Spec.SecretRef.Namespace, cr.Spec.SecretRef.Name, err),
					Guidance: "Could not read the secret — this may be a transient API error or RBAC issue.",
				})
			}
		} else {
			crs.Exists = true
			if !secret.CreationTimestamp.Time.IsZero() {
				crs.Age = time.Since(secret.CreationTimestamp.Time)
			}
			if managedAdminKeyAge > 0 && crs.Age > managedAdminKeyAge {
				crs.NeedsRecreation = true
			}
		}

		report.CredRequests = append(report.CredRequests, crs)
	}

	return nil
}

// isAWSCredReqForDiag is a broader filter for diagnostics — includes all
// CredentialRequests with AWSProviderSpec regardless of name prefix.
func isAWSCredReqForDiag(cr *ccov1.CredentialsRequest) bool {
	if !strings.HasPrefix(cr.Namespace, "openshift") {
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

// ccsSCPActions are representative actions from each service required by the
// OSD CCS minimum SCP (docs.redhat.com/en/documentation/openshift_dedicated/4/html/planning_your_environment/aws-ccs#ccs-aws-scp_aws-ccs).
// These go beyond CredentialRequest-defined permissions and cover the broader
// service access osdCcsAdmin needs as the parent user (e.g., creating IAM users,
// managing autoscaling, CloudWatch monitoring, KMS, Route53, etc.).
var ccsSCPActions = []string{
	"autoscaling:CreateAutoScalingGroup",
	"autoscaling:DescribeAutoScalingGroups",
	"autoscaling:UpdateAutoScalingGroup",
	"autoscaling:DeleteAutoScalingGroup",
	"cloudwatch:GetMetricData",
	"cloudwatch:PutMetricAlarm",
	"cloudwatch:DescribeAlarms",
	"events:PutRule",
	"events:DescribeRule",
	"events:PutTargets",
	"logs:CreateLogGroup",
	"logs:PutLogEvents",
	"logs:DescribeLogGroups",
	"support:DescribeTrustedAdvisorChecks",
	"kms:CreateKey",
	"kms:DescribeKey",
	"kms:Encrypt",
	"kms:Decrypt",
	"sts:AssumeRole",
	"sts:GetCallerIdentity",
	"tag:GetResources",
	"tag:TagResources",
	"route53:CreateHostedZone",
	"route53:ChangeResourceRecordSets",
	"route53:ListHostedZones",
	"servicequotas:ListServices",
	"servicequotas:GetServiceQuota",
	"servicequotas:RequestServiceQuotaIncrease",
}

func diagnosePermissions(ctx context.Context, input *AWSCredsInput, report *DiagnosticReport) error {
	rotationActions := []string{
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

	// Use the resolved username (may have fallen back from suffixed to unsuffixed)
	adminUser := report.ManagedAdminUser
	if adminUser == "" {
		adminUser = input.AdminUsername
	}
	userArn := fmt.Sprintf("arn:aws:iam::%s:user/%s", input.AWSAccountID, adminUser)

	if err := simulateActions(input.AwsClient, userArn, rotationActions, "rotation", nil, report); err != nil {
		return err
	}

	actionToCRs := extractCredReqActions(ctx, input)
	if len(actionToCRs) > 0 {
		var credReqActions []string
		for action := range actionToCRs {
			credReqActions = append(credReqActions, action)
		}
		if err := simulateActions(input.AwsClient, userArn, credReqActions, "credreq", actionToCRs, report); err != nil {
			return err
		}
	}

	if input.IsCCS {
		ccsArn := fmt.Sprintf("arn:aws:iam::%s:user/osdCcsAdmin", input.AWSAccountID)

		ccsActions := make([]string, len(rotationActions))
		copy(ccsActions, rotationActions)
		if len(actionToCRs) > 0 {
			for action := range actionToCRs {
				ccsActions = append(ccsActions, action)
			}
		}
		seen := map[string]bool{}
		var dedupedActions []string
		for _, a := range ccsActions {
			if !seen[a] {
				seen[a] = true
				dedupedActions = append(dedupedActions, a)
			}
		}
		if err := simulateActions(input.AwsClient, ccsArn, dedupedActions, "ccsadmin", nil, report); err != nil {
			return err
		}

		if err := simulateActions(input.AwsClient, ccsArn, ccsSCPActions, "ccs-scp", nil, report); err != nil {
			return err
		}
	}

	if !report.AllPermissionsOK {
		var denied []string
		for _, p := range report.Permissions {
			if !p.Allowed {
				denied = append(denied, p.Action)
			}
		}
		report.Findings = append(report.Findings, Finding{
			Severity: "FAIL",
			Message:  fmt.Sprintf("Insufficient IAM permissions. Denied: %s", strings.Join(denied, ", ")),
			Guidance: "An SCP or IAM policy is restricting required actions. Contact the customer to resolve policy restrictions before rotating credentials.",
		})
	}

	return nil
}

func simulateActions(awsClient awsprovider.Client, policySourceArn string, actions []string, category string, actionToCRs map[string][]string, report *DiagnosticReport) error {
	output, err := awsClient.SimulatePrincipalPolicy(&iam.SimulatePrincipalPolicyInput{
		PolicySourceArn: awsSdk.String(policySourceArn),
		ActionNames:     actions,
	})
	if err != nil {
		report.Findings = append(report.Findings, Finding{
			Severity: "WARN",
			Message:  fmt.Sprintf("Failed to simulate IAM permissions (%s): %v", category, err),
			Guidance: "Permission check could not be completed. This may indicate insufficient access to run SimulatePrincipalPolicy.",
		})
		report.AllPermissionsOK = false
		return nil
	}

	for _, result := range output.EvaluationResults {
		if result.EvalActionName == nil {
			continue
		}
		allowed := result.EvalDecision == iamTypes.PolicyEvaluationDecisionTypeAllowed
		pr := PermissionResult{
			Action:   *result.EvalActionName,
			Allowed:  allowed,
			Category: category,
		}
		if actionToCRs != nil {
			pr.RequestedBy = actionToCRs[*result.EvalActionName]
		}
		report.Permissions = append(report.Permissions, pr)
		if !allowed {
			report.AllPermissionsOK = false
		}
	}

	return nil
}

func extractCredReqActions(ctx context.Context, input *AWSCredsInput) map[string][]string {
	if input.ManagedClient == nil {
		return nil
	}

	actionToCRs := map[string][]string{}
	seen := map[string]map[string]bool{}

	crList := &ccov1.CredentialsRequestList{}
	if err := input.ManagedClient.List(ctx, crList); err != nil {
		return nil
	}

	for i := range crList.Items {
		cr := &crList.Items[i]
		if !isAWSCredReqForDiag(cr) {
			continue
		}
		awsSpec, err := policies.GetAWSProviderSpec(cr)
		if err != nil {
			continue
		}
		for _, stmt := range awsSpec.StatementEntries {
			for _, action := range stmt.Action {
				if seen[action] == nil {
					seen[action] = map[string]bool{}
				}
				if !seen[action][cr.Name] {
					seen[action][cr.Name] = true
					actionToCRs[action] = append(actionToCRs[action], cr.Name)
				}
			}
		}
	}

	return actionToCRs
}

// RenderReport formats the diagnostic report as a human-readable table.
func RenderReport(report *DiagnosticReport, out io.Writer) {
	div := "=========================================================================="

	renderPermissionSummary(report, out)
	fmt.Fprintf(out, "\n%s\n", div)
	fmt.Fprintf(out, " AWS Account: %s    Account CR: %s\n", report.AWSAccountID, report.AccountCRName)
	if report.CallerARN != "" {
		fmt.Fprintf(out, " AWS Caller: %s\n", report.CallerARN)
	}
	fmt.Fprintf(out, "%s\n", div)
	RenderCredRequestTable(report, out)
	fmt.Fprintf(out, "\n%s\n", div)
	renderCredentialsTable(report, out)
	fmt.Fprintf(out, "\n%s\n", div)
	renderSummary(report, out)
	fmt.Fprintf(out, "%s\n\n", div)
}

func underline(width int) string {
	return strings.Repeat("-", width)
}

func renderCredentialsTable(report *DiagnosticReport, out io.Writer) {
	fmt.Fprintf(out, "\nAWS Credentials\n")
	fmt.Fprintf(out, "  %s\n\n", colorBlue("IAM access keys and their corresponding Hive secrets which reference them."))

	if len(report.Keys) == 0 {
		fmt.Fprintf(out, "  (no keys found)\n")
		return
	}

	secretsByKeyID := map[string][]string{}
	for _, s := range report.Secrets {
		if s.Exists && s.AccessKeyID != "" {
			secretsByKeyID[s.AccessKeyID] = append(secretsByKeyID[s.AccessKeyID], s.SecretName)
		}
	}

	fmt.Fprintf(out, "  %-24s %-16s %-6s %-8s %-38s %s\n", "USER", "KEY ID", "AGE", "STATUS", "REF'd by HIVE SECRET(S)", "SYNC")
	fmt.Fprintf(out, "  %-24s %-16s %-6s %-8s %-38s %s\n", underline(24), underline(16), underline(6), underline(8), underline(38), underline(6))

	for _, k := range report.Keys {
		ageStr := formatAge(k.Age)
		secretNames := secretsByKeyID[k.AccessKeyID]
		secretStr := colorBlue("(not referenced by this cluster)")
		syncStr := "--"
		if len(secretNames) > 0 {
			secretStr = strings.Join(secretNames, ", ")
			syncStr = colorGreen("OK")
		}
		if k.HiveMatch && len(secretNames) == 0 {
			syncStr = colorRed("??")
		}
		visibleSecretStr := secretStr
		if len(secretNames) > 0 {
			visibleSecretStr = truncate(secretStr, 38)
		}
		fmt.Fprintf(out, "  %-24s %-16s %-6s %-8s %-38s %s\n",
			truncate(k.UserName, 24),
			truncateKeyID(k.AccessKeyID),
			ageStr,
			k.Status,
			visibleSecretStr,
			syncStr,
		)
	}

	for _, s := range report.Secrets {
		if !s.Exists {
			fmt.Fprintf(out, "\n  %s Hive secret %s/%s not found\n", colorRed("[FAIL]"), s.Namespace, s.SecretName)
		} else if !s.MatchesAWS && s.AccessKeyID != "" {
			fmt.Fprintf(out, "\n  %s Hive secret %s holds key %s which does not match any active AWS key\n",
				colorRed("[FAIL]"), s.SecretName, truncateKeyID(s.AccessKeyID))
		}
	}

}

// DiagnoseCRSecrets produces a lightweight report of just the CredentialRequest
// secrets, using the Hive account secret's modification time for staleness
// detection instead of querying AWS IAM keys.
func DiagnoseCRSecrets(ctx context.Context, hiveClient client.Client, managedClient client.Client, accountCRName string, account *awsv1alpha1.Account, out io.Writer) (*DiagnosticReport, error) {
	report := &DiagnosticReport{
		AWSAccountID:  account.Spec.AwsAccountID,
		AccountCRName: accountCRName,
		IsCCS:         account.Spec.BYOC,
	}

	secretName := accountCRName + "-secret"
	accountSecret := &corev1.Secret{}
	var lastRotationAge time.Duration
	if err := hiveClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: awsAccountNamespace}, accountSecret); err != nil {
		fmt.Fprintf(out, "  %s Could not read Hive account secret %s/%s: %v\n", colorYellow("[WARN]"), awsAccountNamespace, secretName, err)
		fmt.Fprintf(out, "  CR secret staleness detection will be skipped.\n\n")
	} else {
		if !accountSecret.CreationTimestamp.IsZero() {
			lastRotationAge = time.Since(accountSecret.CreationTimestamp.Time)
		}
		if keyID, ok := accountSecret.Data["aws_access_key_id"]; ok {
			fmt.Fprintf(out, "  Active key in Hive: %s (secret age: %s)\n\n", truncateKeyID(string(keyID)), formatAge(lastRotationAge))
		}
	}

	crList := &ccov1.CredentialsRequestList{}
	if err := managedClient.List(ctx, crList); err != nil {
		return nil, fmt.Errorf("failed to list CredentialRequests: %w", err)
	}

	for i := range crList.Items {
		cr := &crList.Items[i]
		if !isAWSCredReqForDiag(cr) {
			continue
		}

		crs := CredRequestStatus{
			CredRequestName: cr.Name,
			SecretName:      cr.Spec.SecretRef.Name,
			Namespace:       cr.Spec.SecretRef.Namespace,
		}

		secret := &corev1.Secret{}
		err := managedClient.Get(ctx, types.NamespacedName{
			Name:      cr.Spec.SecretRef.Name,
			Namespace: cr.Spec.SecretRef.Namespace,
		}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				crs.Exists = false
				crs.ErrorMessage = "not found"
			} else {
				crs.ErrorMessage = fmt.Sprintf("read failed: %v", err)
			}
		} else {
			crs.Exists = true
			if !secret.CreationTimestamp.Time.IsZero() {
				crs.Age = time.Since(secret.CreationTimestamp.Time)
			}
			if lastRotationAge > 0 && crs.Age > lastRotationAge {
				crs.NeedsRecreation = true
			}
		}

		report.CredRequests = append(report.CredRequests, crs)
	}

	return report, nil
}

// RenderCredRequestTable outputs the CredentialRequest secrets table.
func RenderCredRequestTable(report *DiagnosticReport, out io.Writer) {
	fmt.Fprintf(out, "\nCredential Request Secrets (managed cluster)\n")
	fmt.Fprintf(out, "  %s\n", colorBlue("Secrets referenced by AWS CredentialRequests. After rotation, these"))
	fmt.Fprintf(out, "  %s\n", colorBlue("secrets are to be deleted so CCO recreates them with the new credentials."))

	var activeKeyAge string
	for _, k := range report.Keys {
		if k.HiveMatch && k.UserName != "osdCcsAdmin" {
			activeKeyAge = formatAge(k.Age)
			break
		}
	}
	if activeKeyAge != "" {
		fmt.Fprintf(out, "  %s\n", colorBlue(fmt.Sprintf("Secrets older than the active key (%s) were created with a previous key.", activeKeyAge)))
	}
	fmt.Fprintf(out, "\n")

	if len(report.CredRequests) == 0 {
		fmt.Fprintf(out, "  (no credential requests found or managed cluster not connected)\n")
		return
	}

	staleCount := 0
	for _, cr := range report.CredRequests {
		if cr.NeedsRecreation {
			staleCount++
		}
	}

	fmt.Fprintf(out, "  %-40s %-28s %-24s %-8s %s\n", "CREDENTIAL REQUEST", "NAMESPACE", "SECRET", "AGE", "KEY STATUS")
	fmt.Fprintf(out, "  %-40s %-28s %-24s %-8s %s\n", underline(40), underline(28), underline(24), underline(8), underline(14))
	for _, cr := range report.CredRequests {
		ageStr := "--"
		statusStr := colorGreen("CURRENT")
		if !cr.Exists {
			ageStr = "--"
			statusStr = colorRed("MISSING")
		} else {
			if cr.Age > 0 {
				ageStr = formatAge(cr.Age)
			}
			if cr.NeedsRecreation {
				statusStr = colorYellow("NEEDS REFRESH")
			}
		}
		fmt.Fprintf(out, "  %-40s %-28s %-24s %-8s %s\n",
			truncate(cr.CredRequestName, 40),
			truncate(cr.Namespace, 28),
			truncate(cr.SecretName, 24),
			ageStr,
			statusStr,
		)
	}

	if staleCount > 0 {
		fmt.Fprintf(out, "\n  %s %s\n", colorBlue("[INFO]"), colorBlue(fmt.Sprintf("%d secret(s) marked NEEDS REFRESH were provisioned with a previous", staleCount)))
		fmt.Fprintf(out, "         %s\n", colorBlue("access key and will be deleted/recreated during rotation."))
	}

	printInlineFindings(report.Findings, out, "Credential")
}

func renderPermissionSummary(report *DiagnosticReport, out io.Writer) {
	fmt.Fprintf(out, "\nIAM Permission Check (SimulatePrincipalPolicy)\n")
	fmt.Fprintf(out, "  %s\n", colorBlue("Uses SimulatePrincipalPolicy to detect SCP or IAM policy restrictions"))
	fmt.Fprintf(out, "  %s\n", colorBlue("that would block rotation or CCO credential provisioning."))
	if report.CallerARN != "" {
		fmt.Fprintf(out, "  %s\n", colorBlue(fmt.Sprintf("AWS caller (used by this tool): %s", report.CallerARN)))
	}
	fmt.Fprintf(out, "\n")

	if len(report.Permissions) == 0 {
		fmt.Fprintf(out, "  (permission check was not performed)\n")
		return
	}

	hasRotation := false
	hasCredReq := false
	hasCcsAdmin := false
	hasCcsSCP := false
	for _, p := range report.Permissions {
		switch p.Category {
		case "rotation":
			hasRotation = true
		case "credreq":
			hasCredReq = true
		case "ccsadmin":
			hasCcsAdmin = true
		case "ccs-scp":
			hasCcsSCP = true
		}
	}

	fmt.Fprintf(out, "  %s\n\n", colorBlue(fmt.Sprintf("SimulatePrincipalPolicy for user: %s", report.ManagedAdminUser)))

	if hasRotation {
		fmt.Fprintf(out, "  %s\n", colorBlue("Rotation tooling (IAM key management):"))
		fmt.Fprintf(out, "  %-30s %s\n", "ACTION", "RESULT")
		fmt.Fprintf(out, "  %-30s %s\n", underline(30), underline(10))
		for _, p := range report.Permissions {
			if p.Category != "rotation" {
				continue
			}
			result := colorGreen("Allow")
			if !p.Allowed {
				result = colorRed("DENIED")
			}
			fmt.Fprintf(out, "  %-30s %s\n", p.Action, result)
		}
	}

	if hasCredReq {
		fmt.Fprintf(out, "\n  %s\n", colorBlue("CredentialRequest actions (required by cluster operators):"))
		fmt.Fprintf(out, "  %-42s %-10s %s\n", "ACTION", "RESULT", "CREDENTIAL REQUEST(S)")
		fmt.Fprintf(out, "  %-42s %-10s %s\n", underline(42), underline(10), underline(40))
		for _, p := range report.Permissions {
			if p.Category != "credreq" {
				continue
			}
			result := colorGreen("Allow")
			if !p.Allowed {
				result = colorRed("DENIED")
			}
			crNames := "--"
			if len(p.RequestedBy) > 0 {
				crNames = strings.Join(p.RequestedBy, ", ")
			}
			fmt.Fprintf(out, "  %-42s %-10s %s\n", p.Action, result, crNames)
		}
	}

	if hasCcsAdmin || hasCcsSCP {
		fmt.Fprintf(out, "\n  %s\n", colorBlue("SimulatePrincipalPolicy for user: osdCcsAdmin (parent user, manages osdManagedAdmin)"))
	}

	if hasCcsAdmin {
		fmt.Fprintf(out, "\n  %s\n", colorBlue("Rotation + CredentialRequest actions:"))
		fmt.Fprintf(out, "  %-42s %s\n", "ACTION", "RESULT")
		fmt.Fprintf(out, "  %-42s %s\n", underline(42), underline(10))

		allCcsOK := true
		for _, p := range report.Permissions {
			if p.Category != "ccsadmin" {
				continue
			}
			result := colorGreen("Allow")
			if !p.Allowed {
				result = colorRed("DENIED")
				allCcsOK = false
			}
			fmt.Fprintf(out, "  %-42s %s\n", p.Action, result)
		}
		if allCcsOK {
			fmt.Fprintf(out, "\n  %s osdCcsAdmin has required rotation and CR permissions.\n", colorGreen("[OK]"))
		} else {
			fmt.Fprintf(out, "\n  %s osdCcsAdmin is missing required permissions.\n", colorRed("[FAIL]"))
		}
	}

	if hasCcsSCP {
		fmt.Fprintf(out, "\n  %s\n", colorBlue("Additional OSD CCS required services (SCP coverage):"))
		fmt.Fprintf(out, "  %s\n", colorBlue("Ref: docs.redhat.com/en/documentation/openshift_dedicated/4/html/planning_your_environment/aws-ccs#ccs-aws-scp_aws-ccs"))
		fmt.Fprintf(out, "  %-42s %s\n", "ACTION", "RESULT")
		fmt.Fprintf(out, "  %-42s %s\n", underline(42), underline(10))

		allScpOK := true
		for _, p := range report.Permissions {
			if p.Category != "ccs-scp" {
				continue
			}
			result := colorGreen("Allow")
			if !p.Allowed {
				result = colorRed("DENIED")
				allScpOK = false
			}
			fmt.Fprintf(out, "  %-42s %s\n", p.Action, result)
		}
		if allScpOK {
			fmt.Fprintf(out, "\n  %s SCP allows all required OSD CCS services.\n", colorGreen("[OK]"))
		} else {
			fmt.Fprintf(out, "\n  %s SCP may be restricting required services. Review with customer.\n", colorRed("[FAIL]"))
		}
	}

	if report.AllPermissionsOK {
		fmt.Fprintf(out, "\n  %s No SCP or IAM policy restrictions detected.\n", colorGreen("[OK]"))
	} else {
		fmt.Fprintf(out, "\n  %s Some required IAM actions are denied. See findings below.\n", colorRed("[FAIL]"))
	}
}

func renderSummary(report *DiagnosticReport, out io.Writer) {
	fmt.Fprintf(out, "\n Summary:\n")

	rotationPermsOK := true
	credReqPermsOK := true
	ccsPermsOK := true
	scpPermsOK := true
	hasCcs := false
	hasScp := false
	for _, p := range report.Permissions {
		switch p.Category {
		case "rotation":
			if !p.Allowed {
				rotationPermsOK = false
			}
		case "credreq":
			if !p.Allowed {
				credReqPermsOK = false
			}
		case "ccsadmin":
			hasCcs = true
			if !p.Allowed {
				ccsPermsOK = false
			}
		case "ccs-scp":
			hasScp = true
			if !p.Allowed {
				scpPermsOK = false
			}
		}
	}

	if rotationPermsOK && credReqPermsOK {
		fmt.Fprintf(out, " %s %s has adequate permissions to fulfill CRs.\n", colorGreen("[OK]"), report.ManagedAdminUser)
	} else {
		if !rotationPermsOK {
			fmt.Fprintf(out, " %s %s is missing rotation tooling permissions.\n", colorRed("[FAIL]"), report.ManagedAdminUser)
		}
		if !credReqPermsOK {
			fmt.Fprintf(out, " %s %s is missing CredentialRequest permissions.\n", colorRed("[FAIL]"), report.ManagedAdminUser)
		}
	}

	if hasCcs {
		if ccsPermsOK {
			fmt.Fprintf(out, " %s osdCcsAdmin has adequate permissions.\n", colorGreen("[OK]"))
		} else {
			fmt.Fprintf(out, " %s osdCcsAdmin is missing required permissions.\n", colorRed("[FAIL]"))
		}
	}

	if hasScp {
		if scpPermsOK {
			fmt.Fprintf(out, " %s SCP allows all required OSD CCS services.\n", colorGreen("[OK]"))
		} else {
			fmt.Fprintf(out, " %s SCP may be restricting required services.\n", colorRed("[FAIL]"))
		}
	}

	allCRsCurrent := true
	for _, cr := range report.CredRequests {
		if cr.NeedsRecreation || !cr.Exists {
			allCRsCurrent = false
			break
		}
	}
	if allCRsCurrent && len(report.CredRequests) > 0 {
		fmt.Fprintf(out, " %s Cluster CRs are up to date, created after last credential rotation.\n", colorGreen("[OK]"))
	} else if len(report.CredRequests) > 0 {
		staleCount := 0
		for _, cr := range report.CredRequests {
			if cr.NeedsRecreation {
				staleCount++
			}
		}
		if staleCount > 0 {
			fmt.Fprintf(out, " %s %d CR secret(s) need refresh after rotation.\n", colorYellow("[WARN]"), staleCount)
		}
	}

	for _, f := range report.Findings {
		var tag string
		switch f.Severity {
		case "FAIL":
			tag = colorRed("[FAIL]")
		case "WARN":
			tag = colorYellow("[WARN]")
		case "INFO":
			tag = colorBlue("[INFO]")
		default:
			continue
		}
		fmt.Fprintf(out, " %s %s\n", tag, f.Message)
		if f.Guidance != "" {
			for line := range strings.SplitSeq(f.Guidance, "\n") {
				fmt.Fprintf(out, "   -> %s\n", line)
			}
		}
	}

	if report.AllPermissionsOK && report.AllSecretsInSync && allCRsCurrent {
		fmt.Fprintf(out, " No issues found during pre-rotation checks.\n")
	}
}

func printInlineFindings(findings []Finding, out io.Writer, keyword string) {
	for _, f := range findings {
		if strings.Contains(strings.ToLower(f.Message), strings.ToLower(keyword)) {
			fmt.Fprintf(out, "\n  [%s] %s\n", f.Severity, f.Message)
			if f.Guidance != "" {
				for line := range strings.SplitSeq(f.Guidance, "\n") {
					fmt.Fprintf(out, "    -> %s\n", line)
				}
			}
		}
	}
}

func truncateKeyID(keyID string) string {
	if len(keyID) <= 8 {
		return keyID
	}
	return keyID[:4] + "..." + keyID[len(keyID)-4:]
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-2] + ".."
}

func formatAge(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	hours := int(d.Hours())
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

func isNoSuchEntity(err error) bool {
	var nse *iamTypes.NoSuchEntityException
	return errors.As(err, &nse)
}

var (
	colorGreen  = color.New(color.FgGreen).SprintFunc()
	colorYellow = color.New(color.FgYellow).SprintFunc()
	colorRed    = color.New(color.FgRed).SprintFunc()
	colorBlue   = color.New(color.FgCyan).SprintFunc()

	defaultLog = logrus.New()
)

func getLog(l *logrus.Logger) *logrus.Logger {
	if l != nil {
		return l
	}
	return defaultLog
}
